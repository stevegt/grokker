const { execSync } = require("child_process");
const { createWriteStream } = require("fs");
const { chmodSync } = require("fs");
const { pipeline } = require("stream");
const { promisify } = require("util");
const https = require("https");
const path = require("path");
const os = require("os");

const pipelineAsync = promisify(pipeline);
const repo = "stevegt/grokker";  
const version = "v3.0.23"; // XXX Update to match latest release

const platform = os.platform();
const arch = os.arch();
let binaryName = "grok"; // Adjust to your binary name

const getBinaryUrl = () => {
  if (platform === "win32") return `https://github.com/${repo}/releases/download/${version}/${binaryName}.exe`;
  if (platform === "darwin") return `https://github.com/${repo}/releases/download/${version}/${binaryName}-mac`;
  if (platform === "linux") return `https://github.com/${repo}/releases/download/${version}/${binaryName}-linux`;
  throw new Error(`Unsupported platform: ${platform}`);
};

const installBinary = async () => {
  const binaryUrl = getBinaryUrl();
  const binaryPath = path.join(__dirname, binaryName + (platform === "win32" ? ".exe" : ""));

  console.log(`Downloading ${binaryUrl}...`);
  
  const response = await new Promise((resolve, reject) => {
    https.get(binaryUrl, (res) => {
      if (res.statusCode !== 200) {
        reject(new Error(`Failed to download: ${res.statusCode}`));
      }
      resolve(res);
    });
  });

  await pipelineAsync(response, createWriteStream(binaryPath));

  if (platform !== "win32") {
    chmodSync(binaryPath, "755");
  }

  console.log(`Binary installed at ${binaryPath}`);
};

installBinary().catch((err) => {
  console.error("Failed to install binary:", err);
  process.exit(1);
});
