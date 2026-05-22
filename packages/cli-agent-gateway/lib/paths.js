'use strict';

const fs = require('fs');
const path = require('path');
const { BINARY, getPlatformInfo } = require('./platform');

const packageRoot = path.resolve(__dirname, '..');

function readPackageVersion() {
  const pkg = JSON.parse(
    fs.readFileSync(path.join(packageRoot, 'package.json'), 'utf8'),
  );
  return pkg.version;
}

function versionTag() {
  const v = process.env.CG_BINARY_VERSION || readPackageVersion();
  return v.startsWith('v') ? v : `v${v}`;
}

function vendorDir(platformKey) {
  return path.join(packageRoot, 'vendor', platformKey);
}

function installedBinaryPath(platformKey) {
  const { ext } = getPlatformInfo();
  return path.join(vendorDir(platformKey), `${BINARY}${ext}`);
}

function installedVersionFile(platformKey) {
  return path.join(vendorDir(platformKey), 'VERSION');
}

function resolveBinaryPath() {
  if (process.env.CG_BINARY_PATH) {
    return path.resolve(process.env.CG_BINARY_PATH);
  }

  const { platform } = getPlatformInfo();
  const bin = installedBinaryPath(platform);
  if (fs.existsSync(bin)) {
    return bin;
  }

  return null;
}

function githubRepo() {
  return process.env.CG_GITHUB_REPO || 'chaojimct/cli-agent-gateway';
}

module.exports = {
  packageRoot,
  readPackageVersion,
  versionTag,
  vendorDir,
  installedBinaryPath,
  installedVersionFile,
  resolveBinaryPath,
  githubRepo,
};
