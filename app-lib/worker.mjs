import { reloadModule } from 'fusion-hub/functions';
import { isObject, isString } from 'fusion-hub/functions';
import fs from 'fs/promises';
import _fs from 'fs';
import cj from 'color-json';
const logFile = './workspace/output.log';

export async function process(data) {
    if (!_fs.existsSync(logFile))
        await fs.writeFile(logFile,'');    
    let logFileContents = await fs.readFile(logFile, 'utf-8');
    await fs.writeFile(logFile+'.bak',logFileContents);
    let { input, output, app, ai, getUnprocessedActorResponse } = data;
    const actor = await reloadModule("fusion-hub/app-lib/actor");
    const actorParsedResponse = await actor.process({
        getUnprocessedActorResponse,
        isResponseExpected: false,
        ai, app, input, output
    });
    if (ai.chatHistory[0].length < 4) {
        ai.chatHistory[0].splice(2,0, {
            "role": "user",
            "parts": [{
                "text": `Put any requested files into one or more code blocks of the form '${app.settings.workspace}/file.txt' where file.txt is replaced with whatever filename and extension type is approriate.`
            }]
        }, {
            "role": "model",
            "parts": [{
                "text": `Okay. I will put any requested files into the form '${app.settings.workspace}/file.txt', and I will replace 'file.txt' with whatever filename and extension type is approriate. `
            }]
        });
    }
    await fs.writeFile(logFile, logFileContents);
    let workspaceParsedResponse = await actor.process({
        getUnprocessedActorResponse,
        isResponseExpected: false,
        ai, app, input:`Put any requested files into one or more code blocks of the form '${app.settings.workspace}/file.txt' where file.txt is replaced with whatever filename and extension type is approriate. For your response code block, your response short. If you have a question, ask it. Otherwise, say OKAY. If you say OKAY, make sure you also provide the code block that contains the file data. Be sure to keep the code blocks seperated.`, output
    });
    logFileContents+="\n\n"+workspaceParsedResponse['*'];
    await fs.writeFile(logFile, logFileContents);
    delete workspaceParsedResponse['*'];
    output.print(cj(workspaceParsedResponse));

    logFileContents+="\n\n"+actorParsedResponse['*'];
    
    delete actorParsedResponse['*'];
    
    if (isObject(actorParsedResponse) && isObject(actorParsedResponse.response) && isString(actorParsedResponse.response.value) && Object.keys(actorParsedResponse).length==1)
        output.print(actorParsedResponse.response.value);
    else if (isObject(actorParsedResponse)) {
        output.print(cj(actorParsedResponse, null, 2));
    }

}