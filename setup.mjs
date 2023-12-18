import { getChatHistoryDirPath } from 'fusion-hub/paths';
import fs from 'fs/promises';
import _fs from 'fs';

async function init() {
  process.on('uncaughtException', async function(err) {
    //console.log(err.message+"\n");
    if (err.message=='Error processing stream because the response is undefined')
      console.log("\n(No response from AI)\n")
    else
      console.log(err);
  });
  
  if (!_fs.existsSync(await getChatHistoryDirPath())){
    await fs.mkdir(await getChatHistoryDirPath(), { recursive: true });
  }
}

export default {
    init
}