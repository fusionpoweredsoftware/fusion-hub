import _fs from "fs";
import {
  start as aiStart,
  chat
} from "fusion-hub/command/chat-with-ai";
import cli from "fusion-hub/cli";
import {
  fgBlue,
  fgYellow,
  fgGreen,
  reset
} from "fusion-hub/colors";

const { errors: cliErrors } = cli

class Player {
  totalScore = 0;
  pointsReceived = 0;
  usableItems = [];
  itemsUsed = [];
}

export const command = {
  id: "scenario",
  name: "chat with AI",
  description: "Send a message to the AI. Returns the AI's response.",
  examples: `scenario Goggles for safety.`,
  isInputCaseSensitive: true,
  isStartable: true
};
const roundEnd = () => { 
  cli.output.print(`${fgBlue}Round has ended. Progress to next round to continue.${reset}`);
  roundComplete=true;
};
const optionsCount = 9;
let roundNum = 0,
  playerNum = 0,
  playersCount = 2;
let difficulty = 0;
const shuffle = arr => [...arr].sort(() => 0.5 - Math.random());
const allAvailableItems = 'helmet, safety glasses, goggles, face shield, ear protection, dust mask, respirator, gloves, coveralls, hardhat, steel-toed boots, non-slip shoes, life jacket, life preserver, flotation device, fire extinguisher, smoke alarm, carbon monoxide detector, first-aid kit, defibrillator, tourniquet, bandages, gauze, antiseptic, pain reliever, tweezers, scissors, splints, slings, crutches, wheelchairs, canes, screwdriver, hammer, wrench, pliers, tape measure, level, utility knife, flashlight, multi-tool, duct tape, zip ties, bungee cords, carabiners, rope, chain, padlock, tarp, tent, sleeping bag, backpack, water bottle, water filter, water purification tablets, compass, map, lighter, matches, fire starter, whistle, mirror, signaling device, emergency blanket, space blanket, rain poncho, sunglasses, hat, scarf, insect repellent, sunscreen, lip balm, hand sanitizer, toilet paper, tissues, wet wipes, axe, saw, pry bar, crowbar, sledgehammer, pickaxe, shovel, rake, hoe, wheelbarrow, lawnmower, snow shovel, garden hose, sprinkler, telescope, fishing pole, gas stove, lockpick set, sewing kit, gas can, watch, binoculars, portable radio'.toLowerCase().split(', ')
//const allAvailableItems = 'boogey man, ghost, jack skelington, garbage-day man, adam sandler, lawnmower, snow shovel, garden hose, sprinkler'.toLowerCase().split(', ')
let scenario = '',
  roundComplete = false,
  wrongItemUsed = [],
  correction = null,
  solutionItems = []
let players = [];
const game = {
  roundNum: 0,
  playerNum: 0,
  playersCount: 1,
  optionsCount: 9,
  usableItems: [],
  solutionItems: [],
  scenario: '',
  scenario: 0,
  solution: '',
  roundComplete: false,
  totalScore: 0,
  wrongItemUsed: [],
  correction: null
}

const problemThemes = [
  'Mystery-in-a-Mansion',
  'Surviving-a-Deserted-Island',
  'Cybersecurity-Breach',
  'Medical-Outbreak',
  'Time-Travel-Conundrum',
  'Space-Exploration',
  'Post-Apocalyptic-Survival',
  'Environmental-Crisis',
  'Political-Intrigue',
  'Bank-Heist',
  'Art-Theft',
  'Haunted-House',
  'Apartment-Escape',
  'Dystopian-Society',
  'Lost-in-a-Labyrinth',
  'Alien-Invasion',
  'Undercover-Spy',
  'Pirate-Treasure-Hunt',
  'Cyberpunk-City',
  'Olympic-Challenge',
];

function pickRandomStrings(array, count, retainArray = []) {
  if (count >= array.length) {
    return array; // Return the entire array if count exceeds or equals array length
  }

  const availableStrings = array.filter((str) => !retainArray.join(',').replace(/\-/g,' ').split(',').includes(str.replace(/\-/g,' ')));

  if (availableStrings.length < (count-retainArray.length)) {
    // Handle the case where it's impossible to select the desired count of unique strings
    throw new Error('Not enough available strings to pick from.');
  }

  const shuffledArray = shuffle(availableStrings);
  const result = [...retainArray, ...shuffledArray].slice(0, count);
  return result;
}

/*'**Axe**: Can be used to destroy the surveillance camera that is causing the glitch, thus stopping the transmission of the laughter and preventing the disruption of order.\n\n**Tarp**: Can be used to create a makeshift shelter or barrier to hide the laughing citizen from view, buying time to find a more permanent solution.\n\n**Flashlight**: Can be used to signal for help or to distract the authorities while the laughing citizen is hidden or moved to a safer location.\n\n```\n['Axe', 'Tarp', 'Flashlight']\n```'*/

const attemptSolution = async (items, aiChatSettings, additionalInfo='', instructions=null) => {
  const totalAttempts = 5;
  const getResponse = async (instructions) => await chat(instructions || `${scenario}\nIdentify which 3 of following items would best solve the problem in this scenario${additionalInfo}: ${shuffle(items).join(", ")}\nMake sure you explain why the items solve the  double asterisks (**) around each of the 3 items used and ONLY us. At the end of your response, list the 3 items you used as if they were a node JS array, example: ['item1','item2','item3'] `, {
    ...aiChatSettings,
    chatStdout: {
      write: () => {}
    },
    isPersistent: false
  });
  let response,filteredResponse,attemptNum=0, note='';
  const usableItemsSorted = JSON.parse(JSON.stringify(items)).sort((a, b) => b.length - a.length);
  correction=instructions;
  do {
    response = await getResponse(correction);
    filteredResponse = response.split(/[\[\]]/g);
    solutionItems = [];
    if (filteredResponse.length==3) {
      for (let i = 0; i < usableItemsSorted.length; i++) {
        if (!solutionItems.includes(items[i]) && filteredResponse[1].replace(/\-/g,' ').replace(/a[n]? /g,'').replace('the ','').toLowerCase().replace(/[^a-z,`"']/g, ' ').trim().match(new RegExp("['\"`]" + items[i].replace(/\-/g,' ') + "['\"`]", "g"))) {
          solutionItems.push(items[i]);
        }
      }
    } else {
      if (solutionItems.length>3)
        note=' BUT ONLY PICK 3!';
      if (attemptNum>3)
      additionalInfo=`\n(even if it doesn't seem possible; just try your best${note})`;
      
    }
    wrongItemUsed = [];
    for (let i = 0; i < solutionItems.length; i++) {
      if (!items.includes(solutionItems[i])) {
        wrongItemUsed.push(solutionItems[i]);
      }
    }
    if (wrongItemUsed.length > 0)
      correction = `Incorrect. You can't use: ${wrongItemUsed.join(", ")}. You can ONLY use: ${shuffle(items).join(", ")}. Try again.`;
    else
      correction = null;
  } while ((!response || response.length < 32 || filteredResponse.length != 3 || solutionItems.length!=3 || correction) && attemptNum++ < totalAttempts);
  solutionItems=solutionItems.slice(0,3);
  response=filteredResponse[0].replace(/```( )?[^ ]*/g,'').replace(/```( )?/g,'');
  if (attemptNum > totalAttempts) {
    return null;
  }
  return response.trim();
}

export async function start(selectedThemes=null, settings = {}) {
  if (selectedThemes)
    selectedThemes=selectedThemes.split(",");
  playerNum=1;
  players[playerNum] ||= new Player();
  const p = players[playerNum];
  if (typeof settings !== 'object') {
    cliErrors.argumentsLimitError(1); 
    return false;
  }
  const {
    aiChatSettings,
    initParameters = {},
    isNew = false
  } = settings;
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;
  roundComplete = false;
  if (!scenario)
    await aiStart.new({
      initParameters
    });
  else if (!scenario) {
    return;
  }
  if (p.usableItems.length < optionsCount)
    p.usableItems = pickRandomStrings(allAvailableItems, optionsCount - p.usableItems.length, p.usableItems);

  const getNum = () => Math.floor(Math.random() * 9999) + 1;
  const scenarioInsructions = getNum() + `. Provide a short one-sentence ${pickRandomStrings(selectedThemes || problemThemes, 1)[0]} themed scenario with a problem that needs to be solved quickly. Make the scenario as unique as possible but keep it realistic. DO NOT provide the items, ONLY state the problem.`;
  const getScenario = async () => await chat(scenarioInsructions, {
    ...aiChatSettings,
    chatStdout: {
      write: () => {}
      
    }
  });
  scenario = await getScenario();
  let attemptNum = 0;
  while ((!scenario || scenario.length < 16) && attemptNum++ < optionsCount)
    scenario = await getScenario();
  if (scenario=='') {
    cli.output.print(`${fgYellow}Scenario not started.${reset}`)
    return;
  }
  roundNum++;
  cli.output.print(`${fgBlue}Round: ${roundNum}${reset}`);
  if (playersCount > 1)
    cli.output.print(`${fgBlue}Player ${playerNum} is up!${reset}\n`);
  cli.output.print(`${scenario}\n\nThings that player 1 can use to solve the problem (pick 3):\n`);
  for (let i = 1; i < p.usableItems.length + 1; i++) {
    cli.output.print(i + ". " + p.usableItems[i - 1]);
    p.usableItems[i - 1] = p.usableItems[i - 1].toLowerCase().replace(/[^a-z ]/g, ' ').trim();
  }
  cli.output.print();
  return 'scenario';
}

start.new = async (
  themes = null,
  settings = {}
) => {
  if (typeof settings !== 'object') {
    cliErrors.argumentsLimitError(1); 
    return false;
  }
  roundNum = 0;
  players = [];
  scenario = '';
  roundComplete = false;
  wrongItemUsed = [];
  correction = null;
  solutionItems = [];
  await start(themes, settings);
  return 'new scenario';
}

export async function process({
  input,
  settings = {}
}) {
  const {
    aiChatSettings,
    initParameters = {},
    isNew = false
  } = settings;
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;

  if (playerNum>0)
    players[playerNum] ||= new Player();
  const p = players[playerNum];
  const reusableItems = [];
  if (isNew)
    await start.new(settings)
  if (input.length == 0 || (input.length == 1 && (input[0] == 'a') || input[0] == 'ai')) {
    if (input[1] > 7 || input[1] < 0) {
      cli.output.print('Difficulty setting is outside of range. Pick a difficulty from 0 to 7.');
      return;
    }
    if (!p) {
      roundEnd();
      return;
    }
    cli.output.print(`${fgBlue}\nThinking...${reset}\n`);
    p.usableItems = pickRandomStrings(allAvailableItems, optionsCount + (input[1] || difficulty), p.usableItems);
    const result = await attemptSolution(shuffle(p.usableItems).slice(0,9+difficulty), aiChatSettings);
    if (result) {
      p.usableItems = p.usableItems.slice(0,9);
      input=[p.usableItems.indexOf(solutionItems[0]),p.usableItems.indexOf(solutionItems[1]),p.usableItems.indexOf(solutionItems[2])];
    } else {
      cli.output.print(`${fgYellow}Player ${playerNum} has requested a new scenario.${reset}`)
      return;
    }
  }
  if (input.length == 0 || ((input[0] == 'n') || input[0] == 'next')) {
    const themes = input[1];
    await start(themes, settings);
  } else if (input.length <= 4 && input.length > 1 && (input[0] == 'd' || input[0] == 'drop')) {
    if (roundComplete) {
      cli.output.print('You cannot drop items at this time.');
      return;
    }
    if (p.usableItems.length > 6) {
      for (let i = 0; i < p.usableItems.length; i++) {
        if (!input.includes((i + 1).toString())) {
          reusableItems.push(p.usableItems[i]);
        }
      }
      p.usableItems = reusableItems;
      cli.output.print(`\nItems left:\n`);
      for (let i = 1; i < p.usableItems.length + 1; i++) {
        cli.output.print(i + ". " + p.usableItems[i - 1]);
        p.usableItems[i - 1] = p.usableItems[i - 1].toLowerCase().replace(/[^a-z ]/g, ' ').trim();
      }
    } else {
      cli.output.print('Only 3 items can be dropped per round.');
    }
  } else if (scenario) {
    if (playerNum % (playersCount+1) == 0) {
      playerNum = 0;
    }
    if (playerNum==0) {
      roundEnd();
        return;
    } else  {
      cli.output.print(`${fgGreen}Round: ${roundNum}${reset}`);

      p.itemsUsed = [p.usableItems[Number(input[0]) - 1].replace(/[^a-z ]/g, ' '), p.usableItems[Number(input[1]) - 1].replace(/[^a-z ] /g, ' '), p.usableItems[Number(input[2]) - 1].replace(/[^a-z ]/g, ' ')];
      cli.output.print(`Player ${playerNum} used: ${p.itemsUsed}`);
      for (let i = 0; i < p.usableItems.length; i++) {
        if (!input.includes((i + 1).toString())) {
          reusableItems.push(p.usableItems[i]);
        }
      }
      p.usableItems = reusableItems;
      cli.output.print(`\nItems left:\n`);
      for (let i = 1; i < p.usableItems.length + 1; i++) {
        cli.output.print(i + ". " + p.usableItems[i - 1]);
        p.usableItems[i - 1] = p.usableItems[i - 1].toLowerCase().replace(/[^a-z ]/g, ' ').trim();
      }
      if ((++playerNum) % (playersCount+1) == 0) {
        playerNum = 0;
      }
      if (playerNum == 0) {
        cli.output.print();
        cli.output.print(`\n${fgGreen}Recap:${reset}\n`);
        for (let p = 1; p < playersCount+1; p++)
          cli.output.print(`${fgGreen}Player ${p} used: ${players[p].itemsUsed}${reset}`);
        cli.output.print();
        if (!roundComplete) {
          const allItemsUsed=[];
          for (let n = 1; n < playersCount+1; n++) {
            allItemsUsed.push(...players[n].itemsUsed);
          }
          cli.output.print(`${fgBlue}\nThinking...${reset}\n`);
          const solution = await attemptSolution(allItemsUsed, aiChatSettings);
          if (solution)
            cli.output.print(solution);
          else {
            cli.output.print("The game has decided to toss the scenario. No points received this round.");
            return;
          }
        }
        for (let n = 1; n < playersCount+1; n++) {
          let correct = 0, pointsReceived=0;
          for (let s = 0; s < solutionItems.length; s++) {
            if (players[n].itemsUsed.includes(solutionItems[s])) {
                correct++;
            }
          }
          pointsReceived = { 1: 1, 2: 3, 3: 6, 0: 0 }[correct];
          players[n].pointsReceived = pointsReceived;
          players[n].totalScore += pointsReceived;
        }
        let isHandicapGiven=false;
        cli.output.print(`\n${fgGreen}Round: ${roundNum}${reset}`);
        cli.output.print(`\n${fgGreen}Scores:${reset}`);
        for (let n = 1; n < playersCount+1; n++) {
          cli.output.print(`${fgGreen}Player ${n} - ${players[n].totalScore} (+${players[n].pointsReceived})${reset}`);
          if (players[n].pointsReceived==0 && roundNum==1) {
            isHandicapGiven=true;
          }
        }
        if (isHandicapGiven) {
          cli.output.print(`\nFirst-round handicap:`);
          for (let n = 1; n < playersCount+1; n++) {
            if (players[n].pointsReceived==0) {
              players[n].usableItems=pickRandomStrings(allAvailableItems, (optionsCount+1) - players[n].usableItems.length, players[n].usableItems);
              cli.output.print(`${fgBlue}Player ${n} gets an extra item.${reset}`);
            }
          }
        }
        roundEnd();
      } else {
        cli.output.print(`\n${fgBlue}Player ${playerNum} is up!${reset}\n`);
        players[playerNum] ||= new Player();
        const p2 = players[playerNum];
        if (p2.usableItems.length < optionsCount)
          p2.usableItems = pickRandomStrings(allAvailableItems, optionsCount - p2.usableItems.length, p2.usableItems);
        cli.output.print(`${scenario}\n\nThings that player ${playerNum} can use to solve the problem (pick 3):\n`);
        for (let i = 1; i < p2.usableItems.length + 1; i++) {
          cli.output.print(i + ". " + p2.usableItems[i - 1]);
        }
      }
      cli.output.print();
    }
  }
}