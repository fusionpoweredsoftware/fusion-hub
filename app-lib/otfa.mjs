import { expandFilePath, isObject, isString, extractContinuousJapaneseAndChinese } from 'fusion-hub/functions';
import cliColors from 'fusion-hub/colors';
import fs from 'fs/promises';
import _fs from 'fs';
import json5 from 'json5';

export async function process(data) {
    const NONENGLISH_CHARACTERS_DETECTED={};
    const { output, input, getUnprocessedOtfaResponse } = data;

    async function getResponse(explanation) {
        let response = await getUnprocessedOtfaResponse({input,explanation});
        //kludge
        if (extractContinuousJapaneseAndChinese(response).length>0) {
            return NONENGLISH_CHARACTERS_DETECTED;
        }
        return response;
    }

    let response = await getResponse();
    if (isString(response)) {
        output.print(response);
        return response;
    }
    else {
        while (response==NONENGLISH_CHARACTERS_DETECTED) {
            ai.chatHistory[0].pop ();
            ai.chatHistory[0].pop ();
            response = await getResponse("\n\nPlease respond in English only!");
        }
    }
    if (!isString(response)) {
        output.print("Bad response: "+response)
        ai.chatHistory[0].pop ();
        ai.chatHistory[0].pop ();
    }
    return response;
}