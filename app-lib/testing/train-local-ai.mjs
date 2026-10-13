import { isString, isObject, extractContinuousNonEnglishCharacters, readJsonFile } from 'fusion-hub/functions';
import cliColors from 'fusion-hub/colors';
import fs from 'fs/promises';
  
function mapIntegerTo20DHome(integer) {
    //isNeg=false;
    if (integer<=0) {
        //integer=integer.slice(1);
        throw 'Invalid integer: Only non-zero positives allowed.'
        //isNeg=true;
    }
    if (integer > Number.MAX_SAFE_INTEGER) {
       return mapBigIntegerTo20DHome(BigInt(integer));
    }
    const dimensions = new Array(20).fill(0);
    let base = 1;
    let remaining = integer;
  
    for (let i = 0; i < 20; i++) {
      base *= 2; // Increase the base to space out coordinates
      dimensions[i] = (remaining % base); // Multiply by 2 to ensure gaps
      if ((dimensions[i] % 2)==1) {
        dimensions[i] *= -2;
      }
      remaining = Math.floor(remaining / base);
    }
    return dimensions;
  }

  function mapBigIntegerTo20DHome(integer) {
    const dimensions = new Array(20).fill(0);
    let base = 1n;
    let remaining = integer;
  
    for (let i = 0; i < 20n; i++) {
      base *= 2n; // Increase the base to space out coordinates
      dimensions[i] = (remaining % base); // Multiply by 2 to ensure gaps
      if ((dimensions[i] % 2n)==1n) {
        dimensions[i] *= -2n;
      }
      remaining = remaining / base;
    }
  
    return dimensions;
  }
  
  // Assuming mapIntegerTo20DHome is already defined and available


  function nthIterationToTernaryIndex(n) {
    let index = Array(20).fill(0); // Initialize with 20 dimensions
    let i = 0;
    n++;
    while (n > 0) {
        let remainder = n % 3;
        // Map the remainder to -1, 0, 1
        if (remainder == 0) {
            index[i] = 0;
        } else if (remainder == 1) {
            index[i] = 1;
            n -= 1; // Adjust n for the mapped value
        } else { // remainder == 2
            index[i] = -1;
            n += 1; // Adjust n for the mapped value
        }
        n = Math.floor(n / 3);
        i++;
    }
    return index;
}

function mapTokensToNeighborsSequentially(homeValue, tokens, n=0) {
    const homeCoordinates = mapIntegerTo20DHome(homeValue);
    const neighbors = {}; // Use an object to store token mappings
    let currentNeighborIndex = new Array(20).fill(0); // Start at 'home'
    tokens.forEach(token => {
        currentNeighborIndex = nthIterationToTernaryIndex(n++);
        const neighborCoordinates = homeCoordinates.map((coord, i) => coord + currentNeighborIndex[i]);
        const coordinateKey = neighborCoordinates.join(','); // Create a string key from coordinates
        neighbors[simplify(coordinateKey)] = token;
    });

    return { neighbors, n };
}


// Assume mapIntegerTo20DHome, getNextNeighborIndex, and other necessary functions are defined

function simplify(coordinateKey) {
    return coordinateKey.replace(/,((-)?\d+)(,\1)*$/g, ',$1+').replace(/,0\+$/g,'').replace(/,-/g,'-')
    /*.replace(/-1/g,'!').replace(/-2/g,'@').replace(/-3/g,'#').replace(/-4/g,'$').replace(/-5/g,'%').replace(/-6/g,'^').replace(/-7/g,'&').replace(/-8/g,'*').replace(/-9/g,'(')
    
    .replace(/,1/g,'q').replace(/,2/g,'w').replace(/,3/g,'e').replace(/,4/g,'r').replace(/,5/g,'t').replace(/,6/g,'y').replace(/,7/g,'u').replace(/,8/g,'i').replace(/,9/g,'o')
    .replace(/11/g,'a').replace(/22/g,'s').replace(/33/g,'d').replace(/44/g,'f').replace(/55/g,'g').replace(/66/g,'h').replace(/77/g,'j').replace(/88/g,'k').replace(/99/g,'l')
    .replace(/1q/g,'z').replace(/2w/g,'x').replace(/3e/g,'c').replace(/4r/g,'v').replace(/5t/g,'b').replace(/6y/g,'n').replace(/7u/g,'m').replace(/8i/g,',').replace(/9o/g,'.')
                
    .replace(/!q/g,'Q').replace(/@w/g,'W').replace(/#e/g,'E').replace(/\$r/g,'R').replace(/%t/g,'T').replace(/\^y/g,'Y').replace(/&u/g,'U').replace(/\*i/g,'I').replace(/\(o/g,'O')
    .replace(/!a/g,'A').replace(/@s/g,'S').replace(/#d/g,'D').replace(/\$f/g,'F').replace(/%g/g,'G').replace(/\^h/g,'H').replace(/&j/g,'J').replace(/\*k/g,'K').replace(/\(l/g,'L')
    .replace(/!z/g,'Z').replace(/@x/g,'X').replace(/#c/g,'C').replace(/\$v/g,'V').replace(/%b/g,'B').replace(/\^y/g,'N').replace(/&m/g,'M').replace(/\*</g,'<').replace(/\(./g,'>');*/
}

function getTokensBack(homeValue, neighborsMap, startIndex=0) {
    let homeCoordinates = mapIntegerTo20DHome(homeValue);
    let tokens = [];
    let currentNeighborIndex = new Array(20).fill(0); // Start at 'home'
    let index = startIndex;

    // Assuming neighborsMap is now an object with coordinate strings as keys
    while (true) {
        currentNeighborIndex = nthIterationToTernaryIndex(index);
        let neighborCoordinates = homeCoordinates.map((coord, i) => coord + currentNeighborIndex[i]);
        let coordinateKey = neighborCoordinates.join(',');

        if (neighborsMap[simplify(coordinateKey)] === undefined) {
            // Stop if no more tokens are mapped to further neighbors
            break;
        }

        tokens.push(neighborsMap[simplify(coordinateKey)]);
        index++;
    }

    return tokens;
}

export async function process(data) {
    const { ai, app, output, input, getUnprocessedResponse } = data;

    let attempt=0, isBadResponse=false;
    const trainerData = await readJsonFile('~~~/ai-trainer.json', { defaultJson: {}, contextDir: 'ai-trainer' });
    const historyData = await readJsonFile('~~~/.ai-chat-history.json', { defaultJson: {}, contextDir: 'histories' });
    await trainerData.backup();
    await historyData.backup();

    let offset = Math.floor(Number.MAX_SAFE_INTEGER/2);
    
    if (ai.chatHistory[0].length==0) {
        trainerData.json.index||=0;
        trainerData.json.index++;
    }

    let { index } = trainerData.json;
    
    index += offset;
    
    async function getResponse(explanation='') {
        if (attempt>3) {
            isBadResponse=true;
            return;
        }
        attempt++;
        let response = await getUnprocessedResponse({input,explanation});
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
    if (isString(response) && response.length>0) {
        original_response = response;
        if (!isBadResponse) {
            let m1 = mapTokensToNeighborsSequentially(index, ("User: "+input).split(' '), trainerData.json.neighborCount || 0);
            trainerData.json.mappings||={};
            Object.assign(trainerData.json.mappings,m1.neighbors);
            let m2=mapTokensToNeighborsSequentially(index, ("AI: "+response).split(' '),m1.n);
            Object.assign(trainerData.json.mappings,m2.neighbors);
            const originalTokens = getTokensBack(index, trainerData.json.mappings,m1.n+1);
            output.print(`\n${cliColors.fgBlue}AI>${cliColors.reset} `+originalTokens.join(" "));
            trainerData.json.neighborCount=m2.n||0;
        }
    }
    if (isBadResponse || !isString(response) || response.length==0) {
        if (isObject(response)) {
            response = "OBJECT: "+JSON.stringify(response);
        }
        output.print("Bad response.")
        await fs.writeFile('bad_response.txt', `${response}`);
        ai.chatHistory[0].pop();
        ai.chatHistory[0].pop();
    }
    await trainerData.save();
    return response;
}