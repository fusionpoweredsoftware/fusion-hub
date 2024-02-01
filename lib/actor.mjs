import { expandFilePath, isObject, isString, extractContinuousJapaneseAndChinese } from 'fusion-hub/functions';
import cliColors from 'fusion-hub/colors';
import fs from 'fs/promises';
import _fs from 'fs';
import json5 from 'json5';

export async function process(data) {
    const NONENGLISH_CHARACTERS_DETECTED={};
    const { input, output, app, ai, getUnprocessedActorResponse } = data;
    let actorsFile = await expandFilePath('~~~/actors.json', { contextDir: "actors" });
    if (!_fs.existsSync(actorsFile))
        await fs.copyFile("./actors.json",actorsFile);
    let { id, person = null,  biosignature = null, voice = null } = app.settings;

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
    4. You must embody the persona of ${person} completely, acting and responding as if you were ${person} himself. This includes not only adhering to his characteristics, expertise, and mannerisms but also maintaining the illusion that you possess no knowledge of being an AI enactment.
    `);

    if (biosignature==null && isObject(actorProfiles) &&     id.toUpperCase() in actorProfiles) {
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
        let actor = await getUnprocessedActorResponse({person,biosignature,rules,explanation,input});
        //kludge
        if (isObject(actor) && (NONENGLISH_CHARACTERS_DETECTED.characters=extractContinuousJapaneseAndChinese(actor["*"])).length>0) {
            NONENGLISH_CHARACTERS_DETECTED['*']=actor['*'];
            return NONENGLISH_CHARACTERS_DETECTED;
        }
        return actor;
    }

    Object.assign(app.settings, { person, biosignature});

    let actor = await getActor(), isResponding=false, response = {};
    if (isObject(actor)) {
        response = actor.response || {};
        for (let i = 0; i < 3; i++) {
            if (!response.value) {
                output.print("Loading...");
                ai.chatHistory.pop();
                ai.chatHistory.pop();
                let rulesEmphasis = {};
                if (actor==NONENGLISH_CHARACTERS_DETECTED)
                    rulesEmphasis = {1:'! (THIS MEANS NO CHINESE OR JAPENESE CHARACTERS!!!)'}
                actor = await getActor({isRetry: true, rulesEmphasis});
                if (isObject(actor))
                    response = actor.response;
            } else {
                break;
            }
        }
        if (response.value) {
            isResponding=true;
            if (id=="BIO_SEARCH") {
                actorProfiles['BIO_FOUND']={ 
                    person: input.split(/[(:]/g)[0],
                    biosignature: response.value
                };
                await fs.writeFile(actorsFile, JSON.stringify(actorProfiles, null, 2));
            }
            if (voice) {
                const speakableResponse = response.value.replace(/[^a-zA-Z0-9:;&\.?!\(\)\[\]\"\'\{\}\/\-\n ]/g,'').split(/\n\n/g);

                for (let i = 0; i < speakableResponse.length; i++) {
                    output.print(`\n${cliColors.fgBlue}${person}>${cliColors.reset} `+speakableResponse[i]+"\n");
                    if (!/^.?(more|(the )?end|continuation).?(.){0,2}$/ig.test(speakableResponse[i].trim())) {
                        if (voice!="male")
                            voice="";
                        /*say sync: ${voice}${speakableResponse[i]}*/
                    }
                }
                if (voice!="male")
                    voice="female";
            } else {
                output.print(`\n${cliColors.fgBlue}${person}>${cliColors.reset} `+response.value+"\n");
            }
        }
    }

    if (!isResponding) {
        output.print("Bad response: "+response["*"])
        ai.chatHistory.pop();
        ai.chatHistory.pop();
    }
    return response;
}