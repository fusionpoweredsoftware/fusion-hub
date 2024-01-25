//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
async function getActor (notes) {

    /*(actor,TrickyJSON:shortSpokenResponse,nonSpokenResponse) chat-with-ai
    Provide the response of ${person} in the following form:
    {
        "biosignature": "${biosignature}"
        "person": "${person}",
        "input": "${app.input}",
        "shortSpokenResponse": "<put ${person}'s response here>",
        "nonSpokenResponseBaseFilename": "<base filename where the nonSpokenResponse is stored (response.md by default)>",
        "nonSpokenResponseDirectory": "<directory where the nonSpokenResponse is stored ("./" by default)>",
        "isNonSpokenResponseInDiffForm": false,
        "nonSpokenResponse": "<code or anything that is not native to English goes here>",
    }

    Where input, provided as the known given variable in the JSON structure above, is what was said to the actor.
    
    shortSpokenResponse is data that will be passed to a TTS, so do not ever put any code or any non-English characters in that area!

    nonSpokenResponse is where you would put code or not native to English using MD (markdown) format.

    isNonSpokenResponseInDiffForm indicates that the non-spoken response is in diff form. In order to return the isNonSpokenResponseInDiffForm as true, 
    your nonSpokenResponse must looks like this:

    ```json
    [
        {"baseFilename": "<base filename of code block being replaced>", "codeBlockSource": "<first chunk of code that is getting replaced>", "codeBlockReplacement": "<the first chunk of code that will replace the code block source>"},
        {"baseFilename": "<base filename of code block being replaced>", "codeBlockSource": "<second chunk of code that is getting replaced>", "codeBlockReplacement": "<the second chunk of code that will replace the code block source>"},
        {"baseFilename": "<base filename of code block being replaced>", "codeBlockSource": "<third chunk of code that is getting replaced>", "codeBlockReplacement": "<the third chunk of code that will replace the code block source>"},
        ...
    ]  
    ```

    Where "..." signifies the expansion of the json array as needed, to provide all replacement code blocks to effeciently express the changes described in your response.

    IMPORTANT: Whenever there are differences or changes being described, use this diff form to describe the changes rather than providing the entire nonSpokenResponse.

    Also, keep your spoken responses to a minimum. Try to put as much information in your nonSpokenResponse where possible.

    ${notes}
    */
    
    return actor;
}

let { person = "VIDEO", gender = "female", defaultNonSpokenResponseFile = "response.md", workspaceName='./workspace/',notes = '' } = app.settings;
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

if (isObject(actor) && isString(actor.shortSpokenResponse)) {

    let { nonSpokenResponse, nonSpokenResponseDirectory, nonSpokenResponseBaseFilename, isNonSpokenResponseInDiffForm } = actor;
    if (nonSpokenResponseDirectory && !_fs.existsSync(workspaceName+nonSpokenResponseDirectory.replace(/^\.\//g,''))) {
        _fs.mkDirSync(workspaceName, { recursive: true });
    }
    if (nonSpokenResponseBaseFilename) {
        nonSpokenResponseBaseFilename ||= defaultNonSpokenResponseFile;
        nonSpokenResponseBaseFilename = path.join(workspaceName+nonSpokenResponseDirectory.replace(/^\.\//g,''),path.basename(nonSpokenResponseBaseFilename) || defaultNonSpokenResponseFile);
    }
    if (isString(nonSpokenResponse)) {
        await fs.writeFile(workspaceName+(path.basename(nonSpokenResponseBaseFilename) || defaultNonSpokenResponseFile), actor.nonSpokenResponse);
        if (isNonSpokenResponseInDiffForm) {
            nonSpokenResponse = extractJson5Objects(nonSpokenResponse);
        }
    }
    else if (isObject(nonSpokenResponse) || Array.isArray(actor.nonSpokenResponse)) {
        await fs.writeFile(nonSpokenResponseBaseFilename , '```json\n'+json5.stringify(actor.nonSpokenResponse)+'\n```');
    }

    if (Array.isArray(actor.nonSpokenResponse) && isNonSpokenResponseInDiffForm) {
        for (let i = 0; i < nonSpokenResponse.length; i++) {
            let { baseFilename, codeBlockSource, codeBlockReplacement } = actor.nonSpokenResponse;
            if (baseFilename && codeBlockSource && codeBlockReplacement) {
                baseFilename = path.join(workspaceName+nonSpokenResponseDirectory.replace(/^\.\//g,''),path.basename(baseFilename) || defaultNonSpokenResponseFile);
                const fileData = await fs.readFile(filename, 'utf-8');
                fileData=fileData.replace(nonSpokenResponse[i].codeBlockSource,nonSpokenResponse[i].codeBlockReplacement);
                await fs.writeFile(baseFilename, fileData);
            }
        }
    }

    const speakableResponse = actor.shortSpokenResponse.replace(/[^a-zA-Z0-9:;&\.?!\(\)\[\]\"\'\{\}\/\-\n ]/g,'').replace(/[-]/g,' - ').replace(/(^|[^A-Za-z])([A-Z])([A-Z])([A-Z])($|[^A-Za-z])/g,'$1$2.$3.$4.$5').split(/\n\n/g);
    for (let i = 0; i < speakableResponse.length; i++) {
        print((i == 0 ? '\nAI: ' : '') + speakableResponse[i]+"\n")
        /*say sync: ${voiceType}${speakableResponse[i]}*/
    }
} else {
    error("Error processing response.")
}

/*STOP*/
/*start app-using-ai*/