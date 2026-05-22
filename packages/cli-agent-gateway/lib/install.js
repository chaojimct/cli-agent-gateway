'use strict';

const fs = require('fs');
const https = require('https');
const os = require('os');
const path = require('path');
const { spawnSync } = require('child_process');

const { BINARY, getPlatformInfo, releaseAssetNameFor } = require('./platform');
const {
  githubRepo,
  installedBinaryPath,
  installedVersionFile,
  vendorDir,
  versionTag,
} = require('./paths');

function shouldSkipInstall() {
  if (process.env.CG_SKIP_BINARY_INSTALL === '1') {
    return 'CG_SKIP_BINARY_INSTALL=1';
  }
  if (process.env.CG_BINARY_PATH) {
    return 'CG_BINARY_PATH is set';
  }
  if (process.env.npm_config_ignore_scripts === 'true') {
    return 'npm ignore-scripts';
  }
  return null;
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    const request = (target) => {
      https
        .get(target, (res) => {
          if (
            res.statusCode &&
            res.statusCode >= 300 &&
            res.statusCode < 400 &&
            res.headers.location
          ) {
            res.resume();
            request(res.headers.location);
            return;
          }
          if (res.statusCode !== 200) {
            res.resume();
            reject(new Error(`HTTP ${res.statusCode} for ${target}`));
            return;
          }
          res.pipe(file);
          file.on('finish', () => file.close(() => resolve(dest)));
        })
        .on('error', reject);
    };
    request(url);
    file.on('error', reject);
  });
}

function extractArchive(archivePath, destDir, archive) {
  fs.mkdirSync(destDir, { recursive: true });
  if (archive === 'zip') {
    if (process.platform === 'win32') {
      const ps = spawnSync(
        'powershell',
        [
          '-NoProfile',
          '-Command',
          `Expand-Archive -LiteralPath '${archivePath.replace(/'/g, "''")}' -DestinationPath '${destDir.replace(/'/g, "''")}' -Force`,
        ],
        { stdio: 'inherit' },
      );
      if (ps.status !== 0) {
        throw new Error('Expand-Archive failed');
      }
    } else {
      const unzip = spawnSync('unzip', ['-o', archivePath, '-d', destDir], {
        stdio: 'inherit',
      });
      if (unzip.status !== 0) {
        throw new Error('unzip failed');
      }
    }
    return;
  }

  const tar = spawnSync('tar', ['-xzf', archivePath, '-C', destDir], {
    stdio: 'inherit',
  });
  if (tar.status !== 0) {
    throw new Error('tar extract failed');
  }
}

function chmodBinary(binPath) {
  if (process.platform === 'win32') {
    return;
  }
  try {
    fs.chmodSync(binPath, 0o755);
  } catch {
    /* ignore */
  }
}

async function installBinary() {
  const skip = shouldSkipInstall();
  if (skip) {
    console.log(`[cli-agent-gateway] skip binary install (${skip})`);
    return;
  }

  const { platform, archive } = getPlatformInfo();
  const tag = versionTag();
  const destDir = vendorDir(platform);
  const binPath = installedBinaryPath(platform);
  const versionFile = installedVersionFile(platform);

  if (fs.existsSync(binPath) && fs.existsSync(versionFile)) {
    const installed = fs.readFileSync(versionFile, 'utf8').trim();
    if (installed === tag) {
      console.log(`[cli-agent-gateway] binary ${tag} already installed`);
      return;
    }
  }

  const asset = releaseAssetNameFor(platform, tag);
  const url = `https://github.com/${githubRepo()}/releases/download/${tag}/${asset}`;

  console.log(`[cli-agent-gateway] downloading ${asset} ...`);

  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'cag-install-'));
  const archivePath = path.join(tmpRoot, asset);

  try {
    await download(url, archivePath);
    fs.rmSync(destDir, { recursive: true, force: true });
    extractArchive(archivePath, destDir, archive);

    if (!fs.existsSync(binPath)) {
      throw new Error(`binary not found after extract: ${binPath}`);
    }
    chmodBinary(binPath);
    fs.writeFileSync(versionFile, tag, 'utf8');
    console.log(`[cli-agent-gateway] installed ${binPath}`);
  } finally {
    fs.rmSync(tmpRoot, { recursive: true, force: true });
  }
}

module.exports = {
  installBinary,
  shouldSkipInstall,
};
