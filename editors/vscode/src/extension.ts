// The Esquadro extension: the grammar colors .esq files, and esquadro lsp,
// the language server built into esquadro, checks and formats them; the
// panel shows what esquadro gen would do and the conflicts it left.
import * as vscode from "vscode";
import { LanguageClient, LanguageClientOptions, ServerOptions } from "vscode-languageclient/node";
import { Panel } from "./panel";

let client: LanguageClient | undefined;

const esquadro = () => vscode.workspace.getConfiguration("esquadro").get<string>("path") || "esquadro";

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  new Panel(esquadro).register(context);
  const command = esquadro();
  const server: ServerOptions = { command, args: ["lsp"] };
  const options: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "esquadro" }],
    synchronize: {
      // Files changed outside the editor (a git checkout, esquadro gen)
      // are checked again.
      fileEvents: vscode.workspace.createFileSystemWatcher("**/{*.esq,esquadro/lock.json}"),
    },
  };
  client = new LanguageClient("esquadro", "Esquadro", server, options);
  try {
    await client.start();
  } catch (err) {
    client = undefined;
    const install = "How to install";
    const picked = await vscode.window.showErrorMessage(
      `Esquadro: could not run "${command} lsp". Install esquadro, or point esquadro.path at it. (${String(err)})`,
      install,
    );
    if (picked === install) {
      void vscode.env.openExternal(vscode.Uri.parse("https://github.com/patrickluzdev/esquadro#install"));
    }
    return;
  }
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("esquadro.path")) {
        void vscode.window.showInformationMessage("Esquadro: reload the window to use the new esquadro.path.");
      }
    }),
  );
}

export async function deactivate(): Promise<void> {
  await client?.stop();
}
