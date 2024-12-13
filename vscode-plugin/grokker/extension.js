// The module 'vscode' contains the VS Code extensibility API
// Import the module and reference it with the alias vscode in your code below
const vscode = require('vscode');

// This method is called when your extension is activated
// Your extension is activated the very first time the command is executed

/**
 * @param {vscode.ExtensionContext} context
 */

// Create a global terminal variable to be used in the activate function
let terminal;

function activate(context) {

	// Use the console to output diagnostic information (console.log) and errors (console.error)
	// This line of code will only be executed once when your extension is activated
	console.log('Congratulations, your extension "grokker" is now active!');

	// The command has been defined in the package.json file
	// Now provide the implementation of the command with  registerCommand
	// The commandId parameter must match the command field in package.json
	let disposable = vscode.commands.registerCommand('grokker.helloWorld', helloWorld);
	context.subscriptions.push(disposable);

	terminal = vscode.window.createTerminal('aidda-menu');
	let disposable2 = vscode.commands.registerCommand('grokker.aiddaMenu', aiddaMenu);
	context.subscriptions.push(disposable2);
}

// This method is called when your extension is deactivated
function deactivate() {}

module.exports = {
	activate,
	deactivate
}

function helloWorld() {
	vscode.window.showInformationMessage('Hello Again Big World from grokker!');
}

// opens terminal tab running 'grok aidda menu'
function aiddaMenu() {
	terminal.show();
	terminal.sendText('grok aidda menu');
}