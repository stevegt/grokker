// The module 'vscode' contains the VS Code extensibility API
// Import the module and reference it with the alias vscode in your code below
const vscode = require('vscode');
const https = require('https');
const fs = require('fs');
const path = require('path');

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

    let disposable3 = vscode.commands.registerCommand('grokker.installGrokker', async () => {
        try {
            // await downloadGrokkerBinary(context.globalStoragePath);
            await downloadGrokkerBinary(__dirname);
            await vscode.window.showInformationMessage('Grokker binary installed successfully!');
        } catch (error) {
            await vscode.window.showErrorMessage(`Failed to install Grokker: ${error.message}`);
        }
    });
    context.subscriptions.push(disposable3);
}


function helloWorld() {
	vscode.window.showInformationMessage('Hello Again Big World from grokker!');
}

// opens terminal tab running 'grok aidda menu'
function aiddaMenu() {
	terminal.show();
	terminal.sendText('grok aidda menu');
}

// Generate the URL for the latest release of the Grokker binary
const XXXgetBinaryUrl = () => {
  	const repo = "stevegt/grokker";
	const version = "v3.0.23"; // XXX Update to match latest release
	const platform = os.platform();
	const binaryName = 'grok';
	if (platform === "win32") return `https://github.com/${repo}/releases/download/${version}/${binaryName}.exe`;
	if (platform === "darwin") return `https://github.com/${repo}/releases/download/${version}/${binaryName}-mac`;
	if (platform === "linux") return `https://github.com/${repo}/releases/download/${version}/${binaryName}-linux`;
	throw new Error(`Unsupported platform: ${platform}`);
};

// Downloads the Grokker binary from the latest release on GitHub
async function downloadGrokkerBinary(storagePath) {
    const owner = 'stevegt';
    const repo = 'grokker';
	const binaryName = 'grok';

	var apiUrl = `https://api.github.com/repos/${owner}/${repo}/releases/latest`;

    const releaseData = await httpsGetJson(apiUrl);

    const platform = getPlatform();
    // const arch = getArchitecture();
	
	var platformName = platform;
	// the win32 binary has "windows" in the name
	if (platform === 'win32') {
		platformName = 'windows';
	}

    const asset = releaseData.assets.find(asset => 
		// XXX enable arch check
        // asset.name.includes(platform) && asset.name.includes(arch)
        asset.name.includes(platformName)
    );

    if (!asset) {
        throw new Error('No suitable binary found for your system');
    }

	// XXX need to mkdir storagePath if it doesn't exist
    const binaryPath = path.join(storagePath, binaryName + (platform === 'win32' ? '.exe' : ''));
    await downloadFile(asset.browser_download_url, binaryPath);

    // Make the file executable on Unix-like systems
    if (platform !== 'win32') {
        await makeExecutable(binaryPath);
    }
}

function getPlatform() {
    switch (process.platform) {
        case 'win32': return 'windows';
        case 'darwin': return 'darwin';
        case 'linux': return 'linux';
        default: return 'unknown';
    }
}

function getArchitecture() {
    switch (process.arch) {
        case 'x64': return 'amd64';
        case 'ia32': return '386';
        default: return process.arch;
    }
}

function httpsGetJson(url) {
    return new Promise((resolve, reject) => {
        https.get(url, {
            headers: { 'User-Agent': 'VS Code Extension' }
        }, (res) => {
            let data = '';
            res.on('data', (chunk) => data += chunk);
            res.on('end', () => {
                try {
                    resolve(JSON.parse(data));
                } catch (e) {
                    reject(e);
                }
            });
        }).on('error', reject);
    });
}

function downloadFile(url, destPath) {
    return new Promise((resolve, reject) => {
		// XXX creates a zero-length file -- maybe the URL is wrong?
        https.get(url, (res) => {
            const fileStream = fs.createWriteStream(destPath);
            res.pipe(fileStream);
            fileStream.on('finish', () => {
                fileStream.close();
                resolve();
            });
        }).on('error', reject);
    });
}

function makeExecutable(filePath) {
    return new Promise((resolve, reject) => {
        fs.chmod(filePath, '755', (err) => {
            if (err) reject(err);
            else resolve();
        });
    });
}

// This method is called when your extension is deactivated
function deactivate() {}

module.exports = {
    activate,
    deactivate
};

