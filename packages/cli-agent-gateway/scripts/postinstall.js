#!/usr/bin/env node
'use strict';

const { installBinary } = require('../lib/install');

installBinary().catch((err) => {
  console.error('[cli-agent-gateway] postinstall failed:', err.message);
  console.error(
    'Install manually from https://github.com/chaojimct/cli-agent-gateway/releases',
  );
  console.error('or set CG_BINARY_PATH to an existing binary.');
  process.exit(1);
});
