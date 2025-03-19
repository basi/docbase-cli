const fs = require('fs');
const path = require('path');

// Path to cmd/root/root.go file
const rootFilePath = path.join(process.cwd(), 'cmd', 'root', 'root.go');

// Read the file
let content = fs.readFileSync(rootFilePath, 'utf8');

// Regular expression to extract version information
const versionRegex = /Version\s*=\s*"([0-9]+\.[0-9]+\.[0-9]+)"/;
const match = content.match(versionRegex);

if (!match) {
  console.error('Version information not found.');
  process.exit(1);
}

// Current version
const currentVersion = match[1];
console.log(`Current version: ${currentVersion}`);

// Parse version and increment patch version
const [major, minor, patch] = currentVersion.split('.').map(Number);
const newVersion = `${major}.${minor}.${patch + 1}`;
console.log(`New version: ${newVersion}`);

// Update version information
const updatedContent = content.replace(versionRegex, `Version   = "${newVersion}"`);

// Write to file
fs.writeFileSync(rootFilePath, updatedContent);

console.log(`Version updated from ${currentVersion} to ${newVersion}.`);

// Pass version information to GitHub Actions instead of manifest.json
console.log(`::set-output name=version::${newVersion}`);