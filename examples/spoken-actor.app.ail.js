//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
app.inputFileData=null;
async function getActor (notes) {

    /*(actor,TrickyJSON:shortSpokenResponse,nonSpokenResponse) chat-with-ai
    Provide the response of ${person} in the following form:
    {
        "biosignature": "${biosignature}"
        "person": "${person}",
        "input": "${app.input}",
        "inputFileData": ${app.inputFileData}
        "shortSpokenResponse": "<put ${person}'s response here>",
        "nonSpokenResponseBaseFilename": "<base filename where the nonSpokenResponse is stored (response.md by default)>",
        "nonSpokenResponseDirectory": "<directory where the nonSpokenResponse is stored ("./" by default)>",
        "isNonSpokenResponseInDiffForm": false,
        "nonSpokenResponse": "<code or anything that is not native to English goes here>",
    }

    Where input, provided as the known given variable in the JSON structure above, is what was said to the actor.
    
    shortSpokenResponse is data that will be passed to a TTS, so do not ever put any code or any non-English characters in that area!

    nonSpokenResponse is where you would put code or not native to English using MD (markdown) format.

    isNonSpokenResponseInDiffForm indicates that the non-spoken response is in diff form or not. 
    If isNonSpokenResponseInDiffForm is false, respond normally with nonSpokenResponse. I
    
    If isNonSpokenResponseInDiffForm is true, then your nonSpokenResponse must looks like this:

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
    Avoid using isNonSpokenResponseInDiffForm true unless the input specifically asks for differences only.

    Also, keep your spoken responses to a minimum. Try to put as much information in your nonSpokenResponse where possible.

    ${notes}
    */
    
    return actor;
}

function getRandomFillerWord(fillerWords) {
    const randomIndex = Math.floor(Math.random() * fillerWords.length);
    return fillerWords[randomIndex];
}

let { person = "VIDEO", gender = "female", useFillerWords=true, defaultNonSpokenResponseFile = "response.md", inputFile = "input.md", workspaceName='./workspace/',notes = '' } = app.settings;
let { biosignature = null } = app.settings;
let voiceType = '';
if (gender=="male")
    voiceType="(male-voice) ";

if (person === "VIDEO" && biosignature==null) {
    biosignature="I am VIDEO, short for Voice-based IDE Oracle. Think of me as your personal oracle to help you develop your projects more effeciently. It may take some time, and there may be a learning curve, but I believe if we work together we can develop anything!";
    gender = "female";
    notes = "Do not assume any language by default. If you don't know what langauge is preferred then ask.";
}

if (gender=="male")
    voiceType="(male-voice) ";

if (app.input.startsWith("send-file:")) {
    const sendFileArgs = getArgs(app.input.replace(/^send-file:/g,'').trim());
    const inputFilename = sendFileArgs[0];
    const inputFilePath = path.join(workspaceName,path.basename(inputFilename) || defaultInputFile);
    try {
        app.inputFileData = await fs.readFile(inputFilePath, 'utf-8');
    }
    catch (err) {
        error(err);
        return;
    }
    app.inputFileData = `\"${app.inputFileData.replace(/(["\\])/g,'\\$1')}\"`;
    app.input = sendFileArgs.slice(1);
}
else if (app.input.startsWith("send-input:")) {
    const sendInputFilename = app.input.replace(/^send-input:/g,'').trim();
    const inputFilePath = path.join(workspaceName,path.basename(sendInputFilename) || defaultInputFile);
    try {
        app.inputFileData = await fs.readFile(inputFilePath, 'utf-8');
    }
    catch (err) {
        error(err);
        return;
    }   
    app.input = `${app.input.replace(/(["\\])/g,'\\$1')}`;
}


if (useFillerWords==true && ai.chatHistory.length>0 && app.input > 20) {
    const fillerWord = getRandomFillerWord(["Give me a moment to think about that...", "That's very interesting.", "Hmm... I need a minute to process this.", "Okay... let me think about that.", "Um... okay...","Um... alright.", "Well... okay...", "Thinking...", "I'm thinking...", "Well, let me think...", "Hmm... let me think.","Okay. I just need a moment to think about that.", "Give me a minute to think about that.","I'm thinking...","Well, let's see...","Well, let me see...", "Okay, let's see...", "Okay. I'm thinking.", "Give me some time to process that.", "I'll need some time to thank about that."]);
    if (fillerWord) {
        /*say ${voiceType}${fillerWord}*/
    }
}
const actor = await getActor(notes);
print("\nResponding...\n")
if (isObject(actor) && isString(actor.shortSpokenResponse)) {

    let { shortSpokenResponse, nonSpokenResponse, nonSpokenResponseDirectory, nonSpokenResponseBaseFilename, isNonSpokenResponseInDiffForm } = actor;
    if (nonSpokenResponseDirectory && !_fs.existsSync(workspaceName+nonSpokenResponseDirectory.replace(/^\.\//g,''))) {
        _fs.mkDirSync(workspaceName, { recursive: true });
    }
    let nonSpokenResponseFilePath;
    if (nonSpokenResponseBaseFilename) {
        nonSpokenResponseFilePath ||= defaultNonSpokenResponseFile;
        nonSpokenResponseFilePath = path.join(workspaceName+nonSpokenResponseDirectory.replace(/^\.\//g,''),path.basename(nonSpokenResponseBaseFilename) || defaultNonSpokenResponseFile);
    }
    if (nonSpokenResponseFilePath) {
        if (isString(nonSpokenResponse)) {
            if (isNonSpokenResponseInDiffForm) {
                nonSpokenResponse = extractJson5Objects(nonSpokenResponse);
            }
        }
        if ((isObject(nonSpokenResponse) && Object.keys(nonSpokenResponse).length>0) || (Array.isArray(nonSpokenResponse) && nonSpokenResponse.length > 0)) {
            if (isNonSpokenResponseInDiffForm)
                nonSpokenResponseFilePath += ".diff.json";
            else 
                nonSpokenResponseFilePath += ".json";
            await fs.writeFile(nonSpokenResponseFilePath, json5.stringify(nonSpokenResponse, null, 2));
            print("(B) FILE CREATED: "+nonSpokenResponseFilePath);
        }
    } else if (isString(nonSpokenResponse)) {
        print(nonSpokenResponse);
    }

    if (Array.isArray(nonSpokenResponse)) {
        for (let i = 0; i < nonSpokenResponse.length; i++) {
            let { baseFilename, codeBlockSource, codeBlockReplacement } = (nonSpokenResponse[i] ||={});
            if (baseFilename && codeBlockSource && codeBlockReplacement) {
                const filePath = path.join(workspaceName+nonSpokenResponseDirectory.replace(/^\.\//g,''),baseFilename);
                const fileData = await fs.readFile(filePath, 'utf-8');
                fileData=fileData.replace(codeBlockSource,codeBlockReplacement);
                print("FILE UPDATE: "+filePath);
                await fs.writeFile(filePath, fileData);
            }
        }
    }

    const speakableResponse = shortSpokenResponse.replace(/[^a-zA-Z0-9:;&\.?!\(\)\[\]\"\'\{\}\/\-\n ]/g,'').replace(/[-]/g,' - ').replace(/(^|[^A-Za-z])([A-Z])([A-Z])([A-Z])($|[^A-Za-z])/g,'$1$2.$3.$4.$5').split(/\n\n/g);
    for (let i = 0; i < speakableResponse.length; i++) {
        print((i == 0 ? '\nAI: ' : '') + speakableResponse[i]+"\n")
        if (i==speakableResponse.length-1) {
            /*say ${voiceType}${speakableResponse[i]}*/
        } else {
            /*say sync: ${voiceType}${speakableResponse[i]}*/
        }
    }
} else {
    error("Error processing response.")
}

/*STOP*/
/*start app-using-ai*/