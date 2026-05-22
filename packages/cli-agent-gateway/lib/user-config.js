'use strict';

const fs = require('fs');
const os = require('os');
const path = require('path');

const DEFAULT_CONFIG = 'config.yaml';
const LOCAL_CONFIG = 'config.local.yaml';
const DIR_NAME = 'cli-agent-gateway';

const defaultLocalTemplate = `# Local overrides for cli-agent-gateway (user config dir).

cursor:
  binary_path: cursor-agent
  default_model: cursor/composer-2.5-fast
  client_tools_agent_mode: plan
  # proxy: http://127.0.0.1:7890
  # workspace: /path/to/project
`;

function userConfigDir() {
  if (process.env.CG_CONFIG_HOME) {
    return path.resolve(process.env.CG_CONFIG_HOME);
  }
  if (process.platform === 'win32') {
    const appData = process.env.APPDATA || path.join(os.homedir(), 'AppData', 'Roaming');
    return path.join(appData, DIR_NAME);
  }
  const xdg = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config');
  return path.join(xdg, DIR_NAME);
}

function defaultConfigYaml(configDir) {
  const sessions = path.join(configDir, 'sessions.json').replace(/\\/g, '/');
  return `server:
  host: 127.0.0.1
  port: 8080
  read_timeout: 30s
  write_timeout: 300s
  shutdown_timeout: 10s
  max_request_body: 8388608
cursor:
  binary_path: cursor-agent
  default_model: cursor/composer-2.5-fast
  request_timeout: 300s
  max_concurrent: 8
  agent_profile: model
  agent_mode: ask
  client_tools_mode: auto
  client_tools_agent_mode: ask
session:
  enabled: true
  storage_path: ${JSON.stringify(sessions)}
  lock_timeout: 5s
auth:
  enabled: false
logging:
  level: info
  format: json
webui:
  allow_unauthenticated_config: true
  max_traces: 2000
agents:
  auto_discover: true
  default: cursor
`;
}

function ensureUserConfigDir() {
  const dir = userConfigDir();
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });

  const mainPath = path.join(dir, DEFAULT_CONFIG);
  if (!fs.existsSync(mainPath)) {
    fs.writeFileSync(mainPath, defaultConfigYaml(dir), { mode: 0o600 });
  }

  const localPath = path.join(dir, LOCAL_CONFIG);
  if (!fs.existsSync(localPath)) {
    fs.writeFileSync(localPath, defaultLocalTemplate, { mode: 0o600 });
  }

  return dir;
}

/**
 * Inject -config when the user did not pass one (works with older gateway binaries).
 * @param {string[]} args
 * @returns {string[]}
 */
function augmentGatewayArgs(args) {
  if (args.some((a) => a === '-config' || a.startsWith('-config='))) {
    return args;
  }

  const cwdConfig = path.join(process.cwd(), DEFAULT_CONFIG);
  if (fs.existsSync(cwdConfig)) {
    return ['-config', cwdConfig, ...args];
  }

  const dir = ensureUserConfigDir();
  return ['-config', path.join(dir, DEFAULT_CONFIG), ...args];
}

module.exports = {
  userConfigDir,
  ensureUserConfigDir,
  augmentGatewayArgs,
};
