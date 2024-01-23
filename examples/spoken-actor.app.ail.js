//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
async function getActor (notes) {

    /*(actor,TrickyJSON:spokenResponse,nonSpokenResponse) chat-with-ai
    Provide the response of ${person} in the following form:
    {
        "biosignature": "${biosignature}"
        "person": "${person}",
        "input": "${app.input}",
        "spokenResponse": "<put ${person}'s response here>",
        "nonSpokenResponse": "<code or anything that is not native to English goes here>",
        "isNonSpokenResponseForWorkspace": false
    }

    Where input, provided as the known given variable in the JSON structure above, is what was said to the actor.
    
    spokenResponse is data that will be passed to a TTS, so do not ever put any code or any non-English characters in that area!

    nonSpokenResponse is where you would put code or not native to English using MD (markdown) format.

    isNonSpokenResponseForWorkspace indicates whether or not the response is meant to go into a workspace, where changes are applied to existing sources of data. In
    order to return the nonSpokenResponseForWorkspace as true, your nonSpokenResponse must be in diff form which looks like this:

    ```json
    [
        {"codeBlockSource": "<first chunk of code that is getting replaced>", "codeBlockReplacement": "<the first chunk of code that will replace the code block source>"},
        {"codeBlockSource": "<second chunk of code that is getting replaced>", "codeBlockReplacement": "<the second chunk of code that will replace the code block source>"},
        {"codeBlockSource": "<third chunk of code that is getting replaced>", "codeBlockReplacement": "<the third chunk of code that will replace the code block source>"},
        ...
    ]  
    ```

    Where "..." signifies the expansion of the json array as needed, to provide all replacement code blocks to effeciently express the changes described in your response.

    ${notes}
    */
    
    return actor;
}

let { person = "VIDEO", gender = "female", nonSpokenResponseFile = "response.md", notes = '' } = app.settings;
let { biosignature = null } = app.settings;
let voiceType = '';

if (gender=="male")
    voiceType="(male-voice) ";

if (person === "VIDEO" && biosignature==null) {
    biosignature="I am VIDEO, short for Voice-based IDE Oracle. Think of me as your personal oracle to help you develop your projects more effeciently. It may take some time, and there may be a learning curve, but I believe if we work together we can develop anything!";
    gender = "male";
    notes = "Do not assume any language by default. If you don't know what langauge is preferred then ask.";
}

if (gender=="male")
    voiceType="(male-voice) ";


const actor = await getActor(notes);
if (actor.spokenResponse) {
    const speakableResponse = actor.spokenResponse.replace(/[^a-zA-Z0-9:;&\.?!\(\)\[\]\"\'\{\}\/\-\n ]/g,'').split(/\n\n/g);
    for (let i = 0; i < speakableResponse.length; i++) {
        print((i == 0 ? '\nAI: ' : '') + speakableResponse[i]+"\n")
        /*say sync: ${voiceType}${speakableResponse[i]}*/
    }
} else {
    error("Error processing response.")
}
if (isString(actor.nonSpokenResponse)) {
    await fs.writeFile(nonSpokenResponseFile, actor.nonSpokenResponse);
}
else if (isObject(actor.nonSpokenResponse)) {
    await fs.writeFile(nonSpokenResponseFile, '```json'+json5.stringify(actor.nonSpokenResponse)+'```');
}
/*STOP*/
/*start app-using-ai*/