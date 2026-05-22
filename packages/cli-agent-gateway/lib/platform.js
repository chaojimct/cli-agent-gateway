'use strict';

const BINARY = 'cli-agent-gateway';

/**
 * @returns {{ platform: string, ext: string, archive: 'tar.gz' | 'zip' }}
 */
function getPlatformInfo() {
  const { platform, arch } = process;

  if (platform === 'linux' && arch === 'x64') {
    return { platform: 'linux-amd64', ext: '', archive: 'tar.gz' };
  }
  if (platform === 'linux' && arch === 'arm64') {
    return { platform: 'linux-arm64', ext: '', archive: 'tar.gz' };
  }
  if (platform === 'darwin' && arch === 'x64') {
    return { platform: 'darwin-amd64', ext: '', archive: 'tar.gz' };
  }
  if (platform === 'darwin' && arch === 'arm64') {
    return { platform: 'darwin-arm64', ext: '', archive: 'tar.gz' };
  }
  if (platform === 'win32' && arch === 'x64') {
    return { platform: 'windows-amd64', ext: '.exe', archive: 'zip' };
  }
  if (platform === 'win32' && arch === 'arm64') {
    return { platform: 'windows-arm64', ext: '.exe', archive: 'zip' };
  }

  throw new Error(
    `Unsupported platform: ${platform}-${arch}. ` +
      'Install from GitHub Releases or set CG_BINARY_PATH.',
  );
}

function releaseAssetNameFor(platformKey, versionTag) {
  if (platformKey.startsWith('windows-')) {
    return `${BINARY}_${versionTag}_${platformKey}.zip`;
  }
  return `${BINARY}_${versionTag}_${platformKey}.tar.gz`;
}

module.exports = {
  BINARY,
  getPlatformInfo,
  releaseAssetNameFor,
};
