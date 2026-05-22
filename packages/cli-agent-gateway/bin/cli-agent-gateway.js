#!/usr/bin/env node
'use strict';

const { runGateway, runInit, runDoctor } = require('../lib/run');

const args = process.argv.slice(2);

async function main() {
  if (args.length === 0 || args[0] === 'start') {
    const gatewayArgs = args[0] === 'start' ? args.slice(1) : args;
    const code = await runGateway(gatewayArgs);
    process.exit(code);
  }

  if (args[0] === 'init') {
    process.exit(runInit());
  }

  if (args[0] === 'doctor') {
    process.exit(runDoctor());
  }

  if (args[0] === 'help' || args[0] === '--help' || args[0] === '-h') {
    console.log(`cli-agent-gateway — npm wrapper for the Go gateway

Usage:
  cli-agent-gateway [flags]          Start server (same as "start")
  cli-agent-gateway start [flags]    Start server
  cli-agent-gateway init             Initialize user config directory
  cli-agent-gateway doctor           Check binary and environment
  cag                                Short alias

Flags are passed to the Go binary (e.g. -config, -version).

Environment:
  CG_BINARY_PATH           Use a local binary instead of npm vendor/
  CG_SKIP_BINARY_INSTALL   Skip postinstall download
  CG_BINARY_VERSION        Download tag (default: package version)
  CG_GITHUB_REPO           owner/repo for releases
  CG_CONFIG_HOME           User config directory override

Docs: https://github.com/chaojimct/cli-agent-gateway
`);
    process.exit(0);
  }

  const code = await runGateway(args);
  process.exit(code);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
