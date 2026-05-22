'use strict';

const { spawn, spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const { resolveBinaryPath } = require('./paths');
const { augmentGatewayArgs, ensureUserConfigDir, userConfigDir } = require('./user-config');

function missingBinaryHelp() {
  const lines = [
    'cli-agent-gateway binary not found.',
    '',
    'Options:',
    '  1. Re-run install: npm install -g cli-agent-gateway',
    '  2. Set CG_BINARY_PATH to a local binary from GitHub Releases',
    '  3. Download from https://github.com/chaojimct/cli-agent-gateway/releases',
    '',
    'Skip auto-download on install: CG_SKIP_BINARY_INSTALL=1',
  ];
  console.error(lines.join('\n'));
}

function runGateway(args) {
  const bin = resolveBinaryPath();
  if (!bin) {
    missingBinaryHelp();
    return 1;
  }
  if (!fs.existsSync(bin)) {
    console.error(`Binary path does not exist: ${bin}`);
    return 1;
  }

  const gatewayArgs = augmentGatewayArgs(args);

  const child = spawn(bin, gatewayArgs, {
    stdio: 'inherit',
    env: process.env,
    windowsHide: true,
  });

  child.on('error', (err) => {
    console.error(err.message);
    process.exit(1);
  });

  return new Promise((resolve) => {
    child.on('close', (code) => resolve(code ?? 1));
  });
}

function runInit() {
  const dir = ensureUserConfigDir();
  console.log(`Config directory ready:\n  ${dir}`);
  console.log(`  ${path.join(dir, 'config.yaml')}`);
  console.log(`  ${path.join(dir, 'config.local.yaml')}`);
  console.log('Edit config.local.yaml (cursor.binary_path, workspace), then start the gateway.');

  const bin = resolveBinaryPath();
  if (bin && fs.existsSync(bin)) {
    spawnSync(bin, ['init'], { stdio: 'ignore', windowsHide: true });
  }
  return 0;
}

function runDoctor() {
  const bin = resolveBinaryPath();
  if (!bin || !fs.existsSync(bin)) {
    missingBinaryHelp();
    return 1;
  }
  const r = spawnSync(bin, ['doctor'], {
    stdio: 'inherit',
    env: process.env,
    windowsHide: true,
  });
  if (r.status === 0) {
    return 0;
  }
  console.log(`cli-agent-gateway (npm wrapper)`);
  console.log(`binary: ${bin}`);
  console.log(`user config dir: ${userConfigDir()}`);
  return r.status ?? 1;
}

module.exports = {
  runGateway,
  runInit,
  runDoctor,
  missingBinaryHelp,
};
