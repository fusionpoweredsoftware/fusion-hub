import fusionHubApi from 'fusion-hub/api';
import setup from 'fusion-hub/setup';

await setup.init();
await fusionHubApi.preconfigure();
await fusionHubApi.init();
