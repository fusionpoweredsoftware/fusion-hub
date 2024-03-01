import { isString, isObject, extractContinuousNonEnglishCharacters, isJsonString, readJsonFile, generateUniqueId } from 'fusion-hub/functions';
import cliColors from 'fusion-hub/colors';
import fs from 'fs/promises';

const directions = {
    "north": [0, 1, 0],
    "south": [0, -1, 0],
    "east": [1, 0, 0],
    "west": [-1, 0, 0],
    "up": [0, 0, 1],
    "down": [0, 0, -1],
    "northeast": [1, 1, 0],
    "northwest": [-1, 1, 0],
    "southeast": [1, -1, 0],
    "southwest": [-1, -1, 0],
    "up (north)": [0, 1, 1],
    "up (south)": [0, -1, 1],
    "up (east)": [1, 0, 1],
    "up (west)": [-1, 0, 1],
    "up (northeast)": [1, 1, 1],
    "up (northwest)": [-1, 1, 1],
    "up (southeast)": [1, -1, 1],
    "up (southwest)": [-1, -1, 1],
    "down (north)": [0, 1, -1],
    "down (south)": [0, -1, -1],
    "down (east)": [1, 0, -1],
    "down (west)": [-1, 0, -1],
    "down (northeast)": [1, 1, -1],
    "down (northwest)": [-1, 1, -1],
    "down (southeast)": [1, -1, -1],
    "down (southwest)": [-1, -1, -1]
  };

const flippedDirections = {};

for (const [direction, vector] of Object.entries(directions)) {
    const vectorString = vector.toString();
    flippedDirections[vectorString] = direction;
}
  
export async function process(data) {
    const NONENGLISH_CHARACTERS_DETECTED={};
    const { ai, app, output, input, getUnprocessedOtfaResponse, getUnprocessedInvResponse, getUnprocessedResponseVerification, askQuestion } = data;
    const gameData = await readJsonFile('~~~/current-otfa.json', { defaultJson: '{}', contextDir: 'otfa' });

    let attempt=0, isBadResponse=false;

    async function getResponse(explanation='') {
        if (attempt>3) {
            isBadResponse=true;
            return;
        }
        attempt++;
        app.settings.isStart = false;
        
        if (input=="start new game") {
            app.settings.isStart=true;
            gameData.json = {};
            ai.chatHistory[0].length=0;
            
        } else {
            explanation+=`\n\nAt the end of your response provide a positive, neutral, or negative on its own line (- for loss, 0 for nothing, + for gain). This would indicate whether or not the player has made progress toward the end of the game. Do not provide any details about the number, because it will be parsed and removed from the game response using the following method: /\n[-0+]$/g. If you don't follow this regular expression guideline, your response will be rejected. Also, be careful not to award the same action twice. Also keep your response simple in the form of 4 or 5 sentences that form a paragraph as it would appear in a text adventure. Keep sentence structures simple and try to tell a story.`;       
        }
        let type = '';
        if (app.settings.theme)
            type=`${app.settings.theme} themed `;
        
        let response = await getUnprocessedOtfaResponse({input,type,explanation});
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

    async function getResponseIssue(response) {
        let issue = '';
        if (!response.startsWith("You")) {
            return 'The very beginning of your response must match the regex form /^You/g. ^ should not be part of your response; it\'s used to indicate the start of the response for the regex engine.';
        }
        let responseVerification = await getUnprocessedResponseVerification();
        if (isString(responseVerification) && responseVerification) {
            responseVerification = responseVerification.toLowerCase().replace(/[^a-z]/g,'');
            if (/^(yes|no)$/ig.test(responseVerification)) {
                if (responseVerification=="yes") {
                    issue = " \n\nREMEMBER: DO NOT TELL THE PLAYER WHAT OPTIONS THEY HAVE!";
                }
            }  
            ai.chatHistory[0].pop();
            ai.chatHistory[0].pop();
        }
        return issue;
    }

    function generateInventoryList(items) {
        const tally = {};
        items.forEach(item => {
            tally[item] = (tally[item] || 0) + 1;
        });
    
        const inventoryList = [];
        for (const [item, quantity] of Object.entries(tally)) {
            if (quantity > 1) {
                inventoryList.push(`${item} (x${quantity})`);
            } else {
                inventoryList.push(`${item}`);
            }
        }
    
        return inventoryList.join(', ');
    }

    async function verifyInventory() {
        const { json } = gameData;
        json.inventory ||= [];
        json.room ||= {};
        const { inventory } = json;
        json.currentCoordinates ||= '0,0,0';
        let newInventory = await getUnprocessedInvResponse({inventory});
        if (isString(newInventory) && isJsonString(newInventory)) {
            const jsonInventory = JSON.parse(newInventory);
            if (Array.isArray(jsonInventory) && newInventory != inventory.toString()) {
                for (let i = 0; i < json.inventory.length; i++) {
                    if (!jsonInventory.includes(json.inventory[i])) {
                        json.room[json.currentCoordinates] ||= {};
                        json.room[json.currentCoordinates].inventory ||= [];
                        json.room[json.currentCoordinates].inventory.push(json.inventory[i]);
                    }
                }
                json.inventory = jsonInventory;
            }
            ai.chatHistory[0].pop();
            ai.chatHistory[0].pop();
        } else {
            newInventory = await getUnprocessedInvResponse({inventory});
        }
    }

    let response = await getResponse();
    let original_response;
    if (isString(response)) {
        original_response = response;
        if (app.settings.isStart)
            response=response.split(/(You)/g).slice(1).join("").trim().split("\n")[0]+"\n"+original_response.trim().slice(-1);
        else
            response=response.split(/(You)/g).slice(1).join("").replace(/\.[^.]*([-+0])/,'.\n\n$1')
        let issue;
        while (issue = await getResponseIssue(response)) {
            response = await getResponse(issue); 
        }
        ai.chatHistory[0].pop();
        ai.chatHistory[0].push({
            "role": "model",
            "parts": [{
                "text": response
            }]
        });

        while (!/\n[-0+]$/g.test(response) && !isBadResponse) {
            response = await getResponse("\n\n: We need a -, 0, or + on its own line at the end."); 
        }
        if (!isBadResponse)
            output.print(response);
        //return response;
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
    else {
        const { json } = gameData;
        const mainChatHistoryCopy = JSON.stringify(ai.chatHistory[0]);
        await verifyInventory();
        json.room[json.currentCoordinates]||={};

        //Not working yet:
        //
        /*(async () => {
            const genId = generateUniqueId();
            ai.channel=genId;
            ai.chatHistory[ai.channel] = JSON.parse(mainChatHistoryCopy);
            json.room[json.currentCoordinates].name ||= await askQuestion('Has the player moved their position at all?','^(no|\[\-?\d+(\,\s?\-?\d+){2}\])$','ig');
            ai.channel=0;
            delete ai.chatHistory[genId];
        })();
        (async () => {
            const genId = generateUniqueId();
            ai.channel=genId;
            ai.chatHistory[ai.channel] = JSON.parse(mainChatHistoryCopy);
            json.room[json.currentCoordinates].name ||= await askQuestion('What is a short name I can give the room I am in?','[a-zA-Z0-9()&:"\' ]{3,}');
            ai.channel=0;
            delete ai.chatHistory[genId];
        })();
        (async () => {
            const genId = generateUniqueId();
            ai.channel=genId;
            ai.chatHistory[ai.channel] = JSON.parse(mainChatHistoryCopy);
            json.room[json.currentCoordinates].description ||= await askQuestion('Provide a basic description for the room I am in.');
            ai.channel=0;
            delete ai.chatHistory[genId];
        })();*/
    }
    await gameData.save();  
    return response;
}