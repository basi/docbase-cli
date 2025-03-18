const fs = require('fs');
const path = require('path');

// cmd/root/root.goファイルのパス
const rootFilePath = path.join(process.cwd(), 'cmd', 'root', 'root.go');

// ファイルを読み込む
let content = fs.readFileSync(rootFilePath, 'utf8');

// バージョン情報を抽出する正規表現
const versionRegex = /Version\s*=\s*"([0-9]+\.[0-9]+\.[0-9]+)"/;
const match = content.match(versionRegex);

if (!match) {
  console.error('バージョン情報が見つかりませんでした。');
  process.exit(1);
}

// 現在のバージョン
const currentVersion = match[1];
console.log(`現在のバージョン: ${currentVersion}`);

// バージョンをパースして、パッチバージョンをインクリメント
const [major, minor, patch] = currentVersion.split('.').map(Number);
const newVersion = `${major}.${minor}.${patch + 1}`;
console.log(`新しいバージョン: ${newVersion}`);

// バージョン情報を更新
const updatedContent = content.replace(versionRegex, `Version   = "${newVersion}"`);

// ファイルに書き込む
fs.writeFileSync(rootFilePath, updatedContent);

console.log(`バージョンを ${currentVersion} から ${newVersion} に更新しました。`);

// manifest.jsonの代わりにバージョン情報をGitHub Actionsに渡す
console.log(`::set-output name=version::${newVersion}`);