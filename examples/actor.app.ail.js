//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
let actorsFile = await expandFilePath('~~~/actors.json', { contextDir: "actors" });
if (!_fs.existsSync(actorsFile))
    await fs.copyFile("./actors.json",actorsFile);
let { id, person = null,  biosignature = null  } = app.settings;

if (id==null && person==null && biosignature==null) {
    id="EINSTEIN";
}

const actorData = await fs.readFile(actorsFile,'utf-8');
const actorProfiles = json5.parse(actorData);

if (isString(id) && id.includes(":")) {
    const ids=[id.split(":")[0],id.split(":").slice(-1)[0]];
    for (let i = 0; i < ids.length; i++) {
        const _id = ids[i].toUpperCase().trim();
        if (!actorProfiles[_id] && actorProfiles[id]) {
            actorProfiles[_id] = actorProfiles[id];
            if (_id != id)
                delete actorProfiles[id.toUpperCase()];
            await fs.writeFile(actorsFile, JSON.stringify(actorProfiles, null, 2));
            id=_id;
        } else {
            id=ids[i].toUpperCase().trim();
            if (id=='BIO_SEARCH')
                break;
        }
    }
}

const getRules = (emphasis={}) => (
`1. Never use non-English characters${emphasis[1]||'.'}
 2. For your code block response using UIDMD notation, always stay in character${emphasis[2]||'.'}
 3. Always finish your sentence${emphasis[3]||'.'}
`);

if (biosignature==null && isObject(actorProfiles) && id.toUpperCase() in actorProfiles) {
    person||=actorProfiles[id.toUpperCase()].person;
    biosignature||=actorProfiles[id.toUpperCase()].biosignature;
}

async function getActor({isRetry=false, rulesEmphasis}={}) {
    let rules = getRules(rulesEmphasis);
    let explanation='';
    if (ai.chatHistory.length<4 || isRetry) {
        explanation=` When responding, use the following code block with UIDMD notation and populate its contents with the response you are providing for ${person}:
        
        ***plaintext (20240128T061128170Z:response)
        (20240128T061128170Z:response)`;
    }
    /*(actor,UIDMD:20240128T061128170Z,-person,-biosignature,-rules,input) chat-with-ai I have provided you with the person, biosignature, rules, and input code blocks using UIDMD notation. You are to provide the response code block using UIDMD notation, as if you were ${person}.${explanation}*/
    
    //kludge
    if (isObject(actor) && (NONENGLISH_CHARACTERS_DETECTED.characters=extractContinuousJapaneseAndChinese(actor["*"])).length>0) {
        return NONENGLISH_CHARACTERS_DETECTED;
    }
    return actor;
}

let actor = await getActor(), isResponding=false;
if (isObject(actor)) {
    const {response = {} } = actor;
    for (let i = 0; i < 3; i++) {
        if (!response.value) {
            print("Loading...");
            ai.chatHistory.pop();
            ai.chatHistory.pop();
            let rulesEmphasis = {};
            if (actor==NONENGLISH_CHARACTERS_DETECTED)
                rulesEmphasis = {1:'! (THIS MEANS NO CHINESE OR JAPENESE CHARACTERS!!!)'}
            actor = await getActor({isRetry: true, rulesEmphasis});
        } else {
            break;
        }
    }
    if (response.value) {
        print(`\n${cliColors.fgBlue}${person}>${cliColors.reset} `+response.value+"\n");
        isResponding=true;
        if (id=="BIO_SEARCH") {
            actorProfiles['BIO_FOUND']={ 
                person: input.split(/[(:]/g)[0],
                biosignature: response.value
            };
            await fs.writeFile(actorsFile, JSON.stringify(actorProfiles, null, 2));
        }
    }
} 

if (!isResponding) {
    print("Bad response: "+response["*"])
    ai.chatHistory.pop();
    ai.chatHistory.pop();
}

/*STOP*/
/*start app-using-ai*/