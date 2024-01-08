import { pullSetupSettings, pushSetupSettings } from 'fusion-hub/functions';

async function init() {
  process.on('uncaughtException', async function(err) {
    //console.log(err.message+"\n");
    if (err.message=='Error processing stream because the response is undefined')
      console.log("\n(No response from AI)\n")
    else
      console.log(err);
  });
  
  await pullSetupSettings();
}

export default {
    init,
    settings: {},
    pullSetupSettings, 
    pushSetupSettings
}