// What esquadro gen would do to a project, as esquadro gen -dry-run -json
// says: the extension shows it and never works it out on its own.
import { execFile } from "node:child_process"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

export interface Action {
  path: string
  /** create, update, merge, delete, keep or conflict. */
  change: string
  reason?: string
  /** How many conflicts merging it leaves. */
  conflicts?: number
  /** The file on disk has conflicts to resolve. */
  markers?: boolean
  once?: boolean
  blueprint?: string
  template?: string
  /** Its new content, under the plan's out. */
  new?: string
}

export interface Plan {
  esquadro: string
  project: string
  /** same, newer (esquadro upgrade), older or development. */
  versions: string
  notes: { version: string; text: string }[]
  actions: Action[]
  counts: Record<string, number>
  out?: string
}

/** Why esquadro could not say: its message, and whether it is too old. */
export class PlanError extends Error {
  constructor(
    message: string,
    readonly tooOld: boolean,
  ) {
    super(message)
  }
}

/**
 * Asks esquadro what generating the project in dir would do, with the new
 * content of each changed file under a fresh directory (the previous
 * one, if given, is removed).
 */
export async function readPlan(command: string, dir: string, previousOut?: string): Promise<Plan> {
  if (previousOut) await rm(previousOut, { recursive: true, force: true })
  const out = await mkdtemp(join(tmpdir(), "esquadro-plan-"))
  return new Promise((resolve, reject) => {
    execFile(
      command,
      ["gen", "-dry-run", "-json", "-out", out, "."],
      { cwd: dir, maxBuffer: 64 << 20, env: { ...process.env, NO_COLOR: "1" } },
      (err, stdout, stderr) => {
        if (err) {
          void rm(out, { recursive: true, force: true })
          const text = String(stderr || err.message).trim()
          reject(new PlanError(text, /flag provided but not defined: -json/.test(text)))
          return
        }
        try {
          resolve(JSON.parse(stdout) as Plan)
        } catch (e) {
          reject(new PlanError(`esquadro answered what is not a plan: ${String(e)}`, false))
        }
      },
    )
  })
}
