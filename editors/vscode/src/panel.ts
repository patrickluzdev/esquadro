// The Esquadro panel, beside git's: what esquadro gen would do to each
// project in the workspace, the conflicts its merges left, and the
// commands to generate and upgrade.
import { basename, join, relative } from "node:path"
import * as vscode from "vscode"
import { findConflicts, sides } from "./conflicts"
import { type Action, type Plan, PlanError, readPlan } from "./plan"

interface Project {
  dir: string
  plan?: Plan
  error?: PlanError
}

type Node =
  | { kind: "project"; project: Project }
  | { kind: "notice"; project: Project; label: string; detail: string; command?: vscode.Command }
  | { kind: "group"; project: Project; group: Group; actions: Action[] }
  | { kind: "file"; project: Project; action: Action }

interface Group {
  id: string
  label: string
  icon: string
  match: (a: Action) => boolean
}

// In the order a person deals with them.
const GROUPS: Group[] = [
  { id: "resolve", label: "To resolve", icon: "git-merge", match: (a) => !!a.markers },
  { id: "overwrite", label: "Would overwrite your edits", icon: "warning", match: (a) => a.change === "conflict" },
  { id: "merge", label: "To merge into yours", icon: "git-pull-request", match: (a) => a.change === "merge" },
  { id: "update", label: "To update", icon: "diff-modified", match: (a) => a.change === "update" },
  { id: "create", label: "To create", icon: "diff-added", match: (a) => a.change === "create" },
  { id: "delete", label: "To delete", icon: "diff-removed", match: (a) => a.change === "delete" },
  { id: "keep", label: "Yours, kept as they are", icon: "lock", match: (a) => a.change === "keep" },
]

const groupOf = (a: Action) => GROUPS.find((g) => g.match(a))
const SIDE_SCHEME = "esquadro-side"

export class Panel implements vscode.TreeDataProvider<Node> {
  private projects: Project[] = []
  private readonly changed = new vscode.EventEmitter<Node | undefined>()
  readonly onDidChangeTreeData = this.changed.event
  private readonly diagnostics = vscode.languages.createDiagnosticCollection("esquadro")
  private readonly status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50)
  private readonly output = vscode.window.createOutputChannel("Esquadro")
  /** The sides shown in a merge editor, as they were when it opened. */
  private readonly snapshots = new Map<string, string>()
  private timer: NodeJS.Timeout | undefined
  private running: Promise<void> | undefined

  constructor(private readonly command: () => string) {}

  register(context: vscode.ExtensionContext): void {
    const view = vscode.window.createTreeView("esquadro.plan", { treeDataProvider: this, showCollapseAll: true })
    const watcher = vscode.workspace.createFileSystemWatcher("**/esquadro/{*.esq,lock.json,blueprints/**}")
    const later = () => this.refreshSoon()
    watcher.onDidChange(later)
    watcher.onDidCreate(later)
    watcher.onDidDelete(later)
    this.status.command = "esquadro.plan.focus"
    context.subscriptions.push(
      view,
      watcher,
      this.diagnostics,
      this.status,
      this.output,
      this.changed,
      vscode.workspace.registerTextDocumentContentProvider(SIDE_SCHEME, {
        provideTextDocumentContent: (uri) => this.snapshots.get(uri.toString()) ?? "",
      }),
      vscode.workspace.onDidChangeWorkspaceFolders(later),
      vscode.workspace.onDidChangeTextDocument((e) => this.markConflicts(e.document)),
      vscode.workspace.onDidSaveTextDocument((doc) => this.saved(doc)),
      vscode.commands.registerCommand("esquadro.refresh", () => this.refresh()),
      vscode.commands.registerCommand("esquadro.gen", (n?: Node) => this.inTerminal(n, "gen")),
      vscode.commands.registerCommand("esquadro.upgrade", (n?: Node) => this.inTerminal(n, "upgrade")),
      vscode.commands.registerCommand("esquadro.open", (n: Node) => this.open(n)),
      vscode.commands.registerCommand("esquadro.openMerge", (n?: Node | vscode.Uri) => this.openMerge(n)),
      vscode.commands.registerCommand("esquadro.showNotes", (n: Node) => this.showNotes(n.project)),
      vscode.commands.registerCommand("esquadro.showOutput", () => this.output.show()),
    )
    void this.refresh()
  }

  // The tree.

  getTreeItem(n: Node): vscode.TreeItem {
    switch (n.kind) {
      case "project": {
        const item = new vscode.TreeItem(basename(n.project.dir), vscode.TreeItemCollapsibleState.Expanded)
        item.iconPath = new vscode.ThemeIcon("project")
        item.description = this.summary(n.project)
        item.contextValue = "project"
        return item
      }
      case "notice": {
        const item = new vscode.TreeItem(n.label)
        item.tooltip = n.detail
        item.iconPath = new vscode.ThemeIcon("info")
        item.command = n.command
        item.contextValue = "notice"
        return item
      }
      case "group": {
        const item = new vscode.TreeItem(n.group.label, n.group.id === "keep" ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.Expanded)
        item.iconPath = new vscode.ThemeIcon(n.group.icon)
        item.description = String(n.actions.length)
        return item
      }
      case "file": {
        const a = n.action
        const uri = vscode.Uri.file(join(n.project.dir, a.path))
        const item = new vscode.TreeItem(uri)
        item.label = basename(a.path)
        item.description = a.path.includes("/") ? a.path.slice(0, a.path.lastIndexOf("/")) : ""
        if (a.change === "merge" && a.conflicts) item.description += `  ${a.conflicts} conflict${a.conflicts > 1 ? "s" : ""}`
        item.tooltip = [a.path, a.reason, a.template && `template ${a.template} (${a.blueprint})`].filter(Boolean).join("\n")
        item.command = { command: "esquadro.open", title: "Open", arguments: [n] }
        item.contextValue = a.markers ? "conflicted" : "file"
        return item
      }
    }
  }

  getChildren(n?: Node): Node[] {
    if (!n) {
      if (this.projects.length === 1) return this.childrenOf(this.projects[0])
      return this.projects.map((project) => ({ kind: "project", project }))
    }
    if (n.kind === "project") return this.childrenOf(n.project)
    if (n.kind === "group") return n.actions.map((action) => ({ kind: "file", project: n.project, action }))
    return []
  }

  private childrenOf(project: Project): Node[] {
    const out: Node[] = []
    const e = project.error
    if (e) {
      out.push(
        e.tooOld
          ? {
              kind: "notice",
              project,
              label: "This esquadro is too old for the panel: install the latest",
              detail: e.message,
              command: { command: "vscode.open", title: "Install", arguments: [vscode.Uri.parse("https://github.com/patrickluzdev/esquadro#install")] },
            }
          : {
              kind: "notice",
              project,
              label: `esquadro could not plan: ${e.message.split("\n")[0]}`,
              detail: e.message,
              command: { command: "esquadro.showOutput", title: "Show" },
            },
      )
      return out
    }
    const plan = project.plan
    if (!plan) return out
    if (plan.versions === "newer") {
      out.push({
        kind: "notice",
        project,
        label: `Generated with esquadro ${plan.project}: upgrade to ${plan.esquadro}`,
        detail: "Read what changed, then run Upgrade.",
        command: { command: "esquadro.showNotes", title: "What changed", arguments: [{ kind: "notice", project }] },
      })
    } else if (plan.versions === "older") {
      out.push({ kind: "notice", project, label: `Generated with esquadro ${plan.project}, newer than this ${plan.esquadro}: install it`, detail: "" })
    }
    for (const group of GROUPS) {
      const actions = plan.actions.filter((a) => groupOf(a) === group)
      if (actions.length) out.push({ kind: "group", project, group, actions })
    }
    if (!out.length) out.push({ kind: "notice", project, label: "Up to date: generating changes nothing", detail: "" })
    return out
  }

  private summary(p: Project): string {
    if (p.error) return "could not plan"
    const n = p.plan?.actions.filter((a) => a.change !== "keep").length ?? 0
    return n ? `${n} to change` : "up to date"
  }

  // Refreshing.

  refreshSoon(): void {
    clearTimeout(this.timer)
    this.timer = setTimeout(() => void this.refresh(), 400)
  }

  async refresh(): Promise<void> {
    // One at a time: a refresh asked for while one runs runs after it.
    while (this.running) await this.running
    this.running = this.load().finally(() => {
      this.running = undefined
    })
    return this.running
  }

  private async load(): Promise<void> {
    const locks = await vscode.workspace.findFiles("**/esquadro/lock.json", "**/node_modules/**", 50)
    const dirs = locks.map((u) => join(u.fsPath, "..", "..")).sort()
    const previous = new Map(this.projects.map((p) => [p.dir, p]))
    this.projects = await Promise.all(
      dirs.map(async (dir): Promise<Project> => {
        try {
          return { dir, plan: await readPlan(this.command(), dir, previous.get(dir)?.plan?.out) }
        } catch (e) {
          const error = e instanceof PlanError ? e : new PlanError(String(e), false)
          this.output.appendLine(`${dir}: ${error.message}`)
          return { dir, error }
        }
      }),
    )
    await vscode.commands.executeCommand("setContext", "esquadro.hasProject", this.projects.length > 0)
    await this.markAll()
    this.changed.fire(undefined)
  }

  // Conflicts: in Problems, in the status bar, and when saving.

  private async markAll(): Promise<void> {
    this.diagnostics.clear()
    for (const p of this.projects) {
      for (const a of p.plan?.actions ?? []) {
        if (!a.markers) continue
        const uri = vscode.Uri.file(join(p.dir, a.path))
        const doc = await vscode.workspace.openTextDocument(uri).then(
          (d) => d,
          () => undefined,
        )
        if (doc) this.markConflicts(doc, true)
      }
    }
    this.updateStatus()
  }

  private markConflicts(doc: vscode.TextDocument, known = false): void {
    if (doc.uri.scheme !== "file" || (!known && !this.diagnostics.has(doc.uri))) return
    const found = findConflicts(doc.getText())
    this.diagnostics.set(
      doc.uri,
      found.map((c) => {
        const d = new vscode.Diagnostic(
          new vscode.Range(c.start, 0, c.end, doc.lineAt(Math.min(c.end, doc.lineCount - 1)).text.length),
          "A conflict esquadro gen left: keep yours, esquadro's or both, then remove the markers",
          vscode.DiagnosticSeverity.Error,
        )
        d.source = "esquadro"
        return d
      }),
    )
    if (!found.length) this.diagnostics.delete(doc.uri)
    this.updateStatus()
  }

  private updateStatus(): void {
    let conflicts = 0
    this.diagnostics.forEach((_, ds) => {
      conflicts += ds.length
    })
    const upgrade = this.projects.some((p) => p.plan?.versions === "newer")
    if (conflicts) {
      this.status.text = `$(git-merge) Esquadro: ${conflicts} conflict${conflicts > 1 ? "s" : ""}`
      this.status.tooltip = "Conflicts esquadro gen left to resolve"
      this.status.backgroundColor = new vscode.ThemeColor("statusBarItem.warningBackground")
      this.status.show()
    } else if (upgrade) {
      this.status.text = "$(arrow-up) Esquadro: upgrade"
      this.status.tooltip = "A project was generated with an older esquadro"
      this.status.backgroundColor = undefined
      this.status.show()
    } else {
      this.status.hide()
    }
  }

  private async saved(doc: vscode.TextDocument): Promise<void> {
    if (!this.diagnostics.has(doc.uri)) return
    const left = findConflicts(doc.getText()).length
    this.refreshSoon()
    if (!left) return
    const open = "Open in the merge editor"
    const picked = await vscode.window.showWarningMessage(
      `${basename(doc.uri.fsPath)} still has ${left} conflict${left > 1 ? "s" : ""} from esquadro gen.`,
      open,
    )
    if (picked === open) await this.openMerge(doc.uri)
  }

  // Opening a file of the plan.

  private async open(n: Node): Promise<void> {
    if (n.kind !== "file") return
    const a = n.action
    const file = vscode.Uri.file(join(n.project.dir, a.path))
    if (a.markers) return this.openMerge(file)
    const out = n.project.plan?.out
    if (out && a.new) {
      const next = vscode.Uri.file(join(out, a.new))
      const now = a.change === "create" ? vscode.Uri.from({ scheme: SIDE_SCHEME, path: file.path, query: "empty" }) : file
      const title = { create: "new", update: "now ↔ generated", merge: "yours ↔ merged", conflict: "yours ↔ generated" }[a.change] ?? a.change
      return vscode.commands.executeCommand("vscode.diff", now, next, `${basename(a.path)} (${title})`)
    }
    await vscode.commands.executeCommand("vscode.open", file)
  }

  /** The merge editor on a file with conflicts: base, yours, esquadro's. */
  private async openMerge(target?: Node | vscode.Uri): Promise<void> {
    let file: vscode.Uri | undefined
    if (target instanceof vscode.Uri) file = target
    else if (target?.kind === "file") file = vscode.Uri.file(join(target.project.dir, target.action.path))
    else file = vscode.window.activeTextEditor?.document.uri
    if (!file) return
    const doc = await vscode.workspace.openTextDocument(file)
    const s = sides(doc.getText())
    if (!s.conflicts.length) {
      await vscode.window.showInformationMessage(`${basename(file.fsPath)} has no conflicts from esquadro gen.`)
      return
    }
    const side = (name: string, text: string) => {
      const uri = vscode.Uri.from({ scheme: SIDE_SCHEME, path: file.path, query: name })
      this.snapshots.set(uri.toString(), text)
      return uri
    }
    const base = side("base", s.base)
    const yours = side("yours", s.yours)
    const esquadro = side("esquadro", s.esquadro)
    try {
      await vscode.commands.executeCommand("_open.mergeEditor", {
        base,
        input1: { uri: yours, title: "Yours", description: "your file" },
        input2: { uri: esquadro, title: "Esquadro", description: "what the template writes now" },
        output: file,
      })
    } catch {
      // The merge editor's command is the editor's own, and may change:
      // the two sides side by side, then.
      await vscode.commands.executeCommand("vscode.diff", yours, esquadro, `${basename(file.fsPath)} (yours ↔ esquadro)`)
    }
  }

  // Commands.

  private async inTerminal(n: Node | undefined, cmd: "gen" | "upgrade"): Promise<void> {
    const project = n?.project ?? (await this.pickProject())
    if (!project) return
    const terminal = vscode.window.createTerminal({ name: `esquadro ${cmd}`, cwd: project.dir })
    terminal.show()
    terminal.sendText(`${quote(this.command())} ${cmd}`)
    const done = vscode.window.onDidCloseTerminal((t) => {
      if (t === terminal) {
        done.dispose()
        this.refreshSoon()
      }
    })
  }

  private async pickProject(): Promise<Project | undefined> {
    if (this.projects.length <= 1) return this.projects[0]
    const roots = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? ""
    const picked = await vscode.window.showQuickPick(
      this.projects.map((p) => ({ label: basename(p.dir), description: relative(roots, p.dir), project: p })),
      { placeHolder: "Which project?" },
    )
    return picked?.project
  }

  private async showNotes(project: Project): Promise<void> {
    const plan = project.plan
    if (!plan) return
    const text = plan.notes.length
      ? `# From esquadro ${plan.project} to ${plan.esquadro}\n\n${plan.notes.map((n) => n.text.trim()).join("\n\n")}\n`
      : `# From esquadro ${plan.project} to ${plan.esquadro}\n\nNothing changes for the project.\n`
    const doc = await vscode.workspace.openTextDocument({ language: "markdown", content: text })
    await vscode.commands.executeCommand("markdown.showPreview", doc.uri).then(undefined, () => vscode.window.showTextDocument(doc))
    const upgrade = "Upgrade"
    if ((await vscode.window.showInformationMessage(`Upgrade ${basename(project.dir)} to esquadro ${plan.esquadro}?`, upgrade)) === upgrade) {
      await this.inTerminal({ kind: "notice", project, label: "", detail: "" }, "upgrade")
    }
  }
}

/** command as a shell word. */
function quote(command: string): string {
  return /^[\w./:-]+$/.test(command) ? command : `'${command.replaceAll("'", `'\\''`)}'`
}
