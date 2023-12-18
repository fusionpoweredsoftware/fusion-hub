import setup from 'fusion-hub/setup';
import cli, { executeCommand } from 'fusion-hub/cli';

export default {
  init: async (initialInput) => {
    await setup.init();
    await cli.init(initialInput);
  },
  executeCommand,
  cli
}