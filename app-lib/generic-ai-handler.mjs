import { isString, isObject, extractContinuousNonEnglishCharacters } from 'fusion-hub/functions';
import cliColors from 'fusion-hub/colors';
import fs from 'fs/promises';
  
export async function process(data) {
    const { ai, app, output, input, getUnprocessedResponse } = data;

    let attempt=0, isBadResponse=false;

    async function getResponse(explanation='') {
        if (attempt>3) {
            isBadResponse=true;
            return;
        }
        attempt++;
        let response = await getUnprocessedResponse({input,type,explanation});
        if (app.settings.isStart)
            response += "\n0";
        //kludge
        while (extractContinuousNonEnglishCharacters(response).length>0 && !isBadResponse) {
            response = await getResponse("\n\nPlease respond in English-speaking characters only!");
            ai.chatHistory[0].pop();
            ai.chatHistory[0].pop();
        }
        return response;
    }

    let response = await getResponse();
    let original_response;
    if (isString(response)) {
        original_response = response;
        if (!isBadResponse) {
            output.print(`\n${cliColors.fgBlue}AI>${cliColors.reset} `+response);
        }
    }
    if (isBadResponse || !isString(response)) {
        if (isObject(response)) {
            response = "OBJECT: "+JSON.stringify(response);
        }
        output.print("Bad response.")
        await fs.writeFile('bad_response.txt', `${response}`);
        ai.chatHistory[0].pop();
        ai.chatHistory[0].pop();
    }
    return response;
}