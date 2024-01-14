**The adventure game template**
(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
(?:\r?\n?\*/)
stopCommand()
script:

let script={};

// kludge
function isIterable(obj) 
{
    // checks for null and undefined
    if (obj == null) {
        return false;
    }
    return typeof obj[Symbol.iterator] === 'function';
}

async function writeOutGameJson(gameJson)
{
    const jsonString = json5.stringify(gameJson, null, 2);

    await fs.writeFile('gameDefinition.json', jsonString, (err) => {
    if (err) {
        console.error('An error occurred:', err);
        return;
    }
    console.log('JSON5 array saved to file.');
    });
}

let randNum = Math.floor(Math.random()*1000000000000);

/*
setup vertex-ai-project as myvertexai-410504
/*
start new chat-with-ai game-design-conversation-${randNum}.json
*/

script.defaultJSON=`{"theme": "randomly"}`
script.json=JSON.parse(input[1] || script.defaultJSON) 
console.log('hello world')

let N=3;
let E=5;


/*(myStory, text)
We are designing a 1990s style text adventure game (with no graphics right now). Give a game story for a ${script.json.theme} themed text adventure adventure game with a background story followed by ${N} chapters. Write the background story followed by chapter 1 and go to chapter ${N}. The background story section should be titled "Background Story" and after that the first each chapter i should be titled like "Chapter i: <The name of chapter i>".  Each chapter must have an ultimate goal that is further composed of a series of at most ${E} objectives.  Each objective must access a series of rooms (possibly rooms encountered before, new rooms added by solving the previous objective puzzles, or new rooms added by exploring).  The objectives should flow nicely from one chapter to the next. The each objective should be a series of adventure game puzzles.  An adventure game puzzle is some problem that the player encounters that must be solved by some combination of either finding items (or combining items to make new items), using items to accomplish game objectives, talking to characters in the game, or exploring by going to new rooms. (Some items can be picked up and put in a player's inventory.  Just like in the 1990s style adventure games, the players can have an unrealistically large inventory they can carry around with them.) Adventure game puzzles may only depend on the prior completion of puzzles in the current objective (you can assume all of the puzzles in the previous objectives have been solved).  Puzzles must be solved in a specific order. Rooms, in and of themselves must either physical locations (like in 1990s style adventure games) or cut scenes, such as a long journey.  Rooms must be connected in some way to other rooms.  For now, the chapters should be relatively short (1 paragraph for each objective) but you will expand each chapter later in the following queries (not now).  The game play will actually start after the background story, so just keep that in mind when you write everything out. In the final chapter, chapter ${N}, the game should culminate in some kind of grand accomplishment achieved such that the player feels like they have won the game.  Write the background story followed by chapter 1 and go to chapter ${N} now.
/*(myGameJSON, JSON)
Put the story design into a JSON5 (no quotes around variables) object of the form:

{
    title: <title_string>, 
    gameBackgroundStory: <game_background_story_string>,
    chapters=[null,
        { chapterTitle: <chapter_1_title_string>, chapterText: <chapter_1_text_string> },
        { chapterTitle: <chapter_2_title_string>, chapterText: <chapter_2_text_string> },
        ...,
        { chapterTitle: <chapter_${N}_title_string>, chapterText: <chapter_${N}_text_string> }
    ]
}

Make sure you do *ALL* of the chapters.  Note, only in this case I put a null to skip the 0th array entry.  Do not expand the chapter yet.  Just put it in this JSON5 (no quotes around variables) format verbatim.  The chapterText string must contain *all* of the text you generated for the chapter from your previous response copied verbatim for each chapter.  The point of this is simply to put your previous response into JSON5 form.
*/

// kludge
if (myGameJSON == null || !myGameJSON.chapters || !isIterable(myGameJSON.chapters) || myGameJSON.chapters.length == 0)
    myGameJSON = await repeatCommand();

// Loop over Chapters
for (let i=1; i<=N; i++)
{
    /*(chapterJson, JSON)
    Here you will expand out each chapter.  Chapter objective and the objectives of chapter ${i}, entitled ${myGameJSON.chapters[i].chapterTitle}, into a JSON5 (no quotes around variables) object of the form:
    
    {
        chapterObjective: <chapter_objective_string>
        objectives: [ 
            {
                objectiveDescription: <objective_description_string>,
                pointsGains: <number_of_points_gained>
            },
            {
                objectiveDescription: <objective_description_string>,
                pointsGains: <number_of_points_gained>
            },...
        ]
    } 
    
    Note here I use "..." to indicate there may be multiple JSON5 (no quotes around variables) objects in an array.  Also, note objectives is an array of JSON5 (no quotes around variables) objects. Do not leave out any objectives.
    */

    myGameJSON.chapters[i].chapterDetails=chapterJson

    // kludge
    if (myGameJSON == null || !chapterJson.objectives || !isIterable(chapterJson.objectives) || chapterJson.objectives.length == 0)
        myGameJSON = await repeatCommand();


    // Loop over objectives
    for (var objective of chapterJson.objectives)
    {
        /*(objectiveRoomDetailsJson, JSON)
        For chapter ${i}, ${myGameJSON.chapters[i].chapterTitle}, and objective ${objective.objectiveDescription}, create a JSON5 (no quotes around variables) object containing an array, called newRooms, with details of each new room unlocked by entering the specified chapter and objective in the form:
        
        {
            newRooms: [
                { 
                    roomName: <room_name_string>,
                    roomDescription: <room_description_string>, 
                    isLongJourney: <boolean>,
                    outgoingConnectedRooms: [<connectedRoomNameString>,<connectedRoomNameString>,... ]
                    pointsGains: <number_of_points_gained> 
                },
                { 
                    roomName: <room_name_string>,
                    roomDescription: <room_description_string>, 
                    isLongJourney: <boolean>,
                    outgoingConnectedRooms: [<connectedRoomNameString>,<connectedRoomNameString>,... ]
                    pointsGains: <number_of_points_gained> 
                }, 
                ...
            ]
        }
            
        Note, here I use "..." to indicate there may be multiple JSON5 (no quotes around variables) objects in an array. You may not include "..." in your response.  You must completely fill in the arrays.  Where all of the adventure game rooms for the specified chapter and objective are stored. Note, players are not aware of these new rooms until they are explored.  Connected rooms should be physically nearby or (very occasionally) refer to rooms that are long journeys taken from the current room.  Long journey rooms will always have only 1 outgoing room which the player will be taken to automatically after the long journey. The isLongJourney flag should only be true if the room actually refers to a long journey, otherwise the isLongJourney flag should be false. Each room should have on average 4 connected rooms, but some may have 1 or 2 or 3 or 5, or 6. New room JSON5 objects must include roomName, roomDescription, isLongJourney, and outgoingConnectedRooms. You cannot leave out any of these properties. Again, each room in the array must be in this format:
        
        { 
            roomName: <room_name_string>,
            roomDescription: <room_description_string>, 
            isLongJourney: <boolean>,
            outgoingConnectedRooms: [<connectedRoomNameString>,<connectedRoomNameString>,... ]
            pointsGains: <number_of_points_gained> 
        }
        
        Also, you must include *all* of the new rooms necessary to complete the objective.
        */

        //objectiveRoomDetailsJson = {
        //}

        // kludge
        if (objectiveRoomDetailsJson == undefined || objectiveRoomDetailsJson == null || !objectiveRoomDetailsJson.newRooms || !isIterable(objectiveRoomDetailsJson.newRooms) || objectiveRoomDetailsJson.newRooms.length == 0)
            objectiveRoomDetailsJson = await repeatCommand();

        
        objective.objectiveDetails = objectiveRoomDetailsJson


        /*(objectivePuzzleDetailsJson, JSON)
        For chapter ${i}, ${myGameJSON.chapters[i].chapterTitle}, and objective ${objective.objectiveDescription}, create a JSON5 (no quotes around variables) object containing an array, called puzzles, with details of each new room unlocked by entering the specified chapter and objective in the form:
        
        {
            puzzles: [
                {
                    puzzleName: <puzzle_name_string>.
                    puzzleDescription: <detailed_puzzle_description_string>.
                    pointsGains: <number_of_points_gained> 
                },
                {
                    puzzleName: <puzzle_name_string>.
                    puzzleDescription: <detailed_puzzle_description_string>.
                    pointsGains: <number_of_points_gained> 
                },
                ...
            ]
        }
            
        Note here I use "..." to indicate there may be multiple JSON5 (no quotes around variables) objects in an array. You may not include "..." in your response.  You must completely fill in the array.  Puzzles is an array of JSON5 (no quotes around variables) objects containing all of the puzzles for this objective. The <puzzle_name_string> is the puzzle name.  The <detailed_puzzle_description_string> is a detailed description of the puzzle and the objectives that must be accomplished to solve the puzzle.  Puzzles must be solved in the order they are listed here. Puzzle JSON5 objects must contain puzzleName and puzzleDescription. You cannot leave out any of these properties.  You must include *all* of the puzzles necessary to complete the objective. 
        */

        // kludge
        if (objectivePuzzleDetailsJson == null || !objectivePuzzleDetailsJson.puzzles || !isIterable(objectivePuzzleDetailsJson.puzzles) || objectivePuzzleDetailsJson.puzzles.length == 0)
            objectivePuzzleDetailsJson = await repeatCommand();


        objective.objectiveDetails.puzzles = objectivePuzzleDetailsJson.puzzles


        for (var room of objective.objectiveDetails.newRooms)
        {
            /*(roomItemsJson, JSON)
            For chapter ${i}, ${myGameJSON.chapters[i].chapterTitle}, and objective  ${objective.objectiveDescription}, and room ${room.roomName} provide an array of all of the items that can originally be found in this room in the form:
            
            {
                items: [
                    {
                        name: <item_name_string>,
                        description: <item_description_string>,
                        commandList: [
                            {
                                command: <command_description_string>,
                                prerequisites: [<prerequisite_description_string>,<prerequisite_description_string>,...],
                                gameWorldStateChange: [<state_change_string>,<state_change_string>,...],
                                pointsGains: <number_of_points_gained> 
                            },
                            {
                                command: <command_description_string>,
                                prerequisites: [<prerequisite_description_string>,<prerequisite_description_string>,...],
                                gameWorldStateChange: [<state_change_string>,<state_change_string>,...],
                                pointsGains: <number_of_points_gained>
                            },
                            ...
                        ]
                    },
                    {
                        name: <item_name_string>,
                        description: <item_description_string>,
                        commandList: [
                            {
                                command: <command_description_string>,
                                prerequisites: [<prerequisite_description_string>,<prerequisite_description_string>,...],
                                gameWorldStateChange: [<state_change_string>,<state_change_string>,...],
                                pointsGains: <number_of_points_gained>
                            },
                            {
                                command: <command_description_string>,
                                prerequisites: [<prerequisite_description_string>,<prerequisite_description_string>,...],
                                gameWorldStateChange: [<state_change_string>,<state_change_string>,...],
                                pointsGains: <number_of_points_gained> 
                            },
                            ...
                        ]
                    },
                    ...
                ]
            }

            Here I gave an example of one item but there may be multiple items in an array so I put "..." to signify that.  You may not include "..." in your response.  You must completely fill in the arrays.   The <item_name_string> is the name of the item that can be found in the room, <item_description_string> is a description of the item. The command list contains a list of JSON5 (no quotes around variables) objects each with a command defined by <command_description_string> containing the actions the player has to perform with the item to make progress in the game with it.  If an item can be picked up and put in your inventory you must create a command JSON5 (no quotes around variables) object for that and add it to the commandList.  Every action must be a separate command.  Do not combine separate actions into one command.  The commands must correspond to the objective.  The prerequisites array is a list of prerequisites required to perform a command.  For example, if a specific puzzle needs to be completed first to perform the command, then it must be listed as a prerequisite or if another item is needed to perform the action it must be listed as a prerequisite.  Make sure you list prerequisites in the order they must be accomplished in the game.  Items JSON5 objects must contain name, description, and commandList. You cannot leave out any of these properties.  The commandList JSON5 objects (if they exist), must contain command, prerequisites, gameWorldStateChange, and pointsGains.   You cannot leave out any of these properties.   You must include *all* of the items necessary that start out in this room.  You may also include some unnecessary items to make the game more challenging.
            */
            
            // kludge
            if (roomItemsJson == null || !roomItemsJson.items || !isIterable(roomItemsJson.items) || roomItemsJson.items.length == 0)
                roomItemsJson = await repeatCommand();


            room.items = roomItemsJson.items

            /*(roomCharactersJson, JSON)
            For chapter ${i}, ${myGameJSON.chapters[i].chapterTitle}, and objective  ${objective.objectiveDescription}, and room ${room.roomName} provide an array of unique characters that can be found in this room in the form:
            
            {
                characters: [
                    {
                        name: <character_name_string>,
                        description: <character_description_string>,
                        conditions: <description_of_the_conditions_for_finding_character_this_specific_room>,
                        characterTalkingTopics: [
                            {
                                topic: <topic>,
                                characterSays: <what_character_says>,
                                prerequisites: [<prerequisite_description_string>,<prerequisite_description_string>,...],
                                gameWorldStateChange: [<state_change_string>,<state_change_string>,...],
                                pointsGains: <number_of_points_gained>
                            },
                            {
                                topic: <topic>,
                                characterSays: <what_character_says>,
                                prerequisites: [<prerequisite_description_string>,<prerequisite_description_string>,...],
                                gameWorldStateChange: [<state_change_string>,<state_change_string>,...],
                                pointsGains: <number_of_points_gained>
                            },   
                            ...
                        ]
                    },
                    {
                        name: <character_name_string>,
                        description: <character_description_string>,
                        conditions: <description_of_the_conditions_for_finding_character_this_specific_room>,
                        characterTalkingTopics: [
                            {
                                topic: <topic>,
                                characterSays: <what_character_says>,
                                prerequisites: [<prerequisite_description_string>,<prerequisite_description_string>,...],
                                gameWorldStateChange: [<state_change_string>,<state_change_string>,...],
                                pointsGains: <number_of_points_gained>
                            },
                            {
                                topic: <topic>,
                                characterSays: <what_character_says>,
                                prerequisites: [<prerequisite_description_string>,<prerequisite_description_string>,...],
                                gameWorldStateChange: [<state_change_string>,<state_change_string>,...],
                                pointsGains: <number_of_points_gained>
                            },   
                            ...
                        ]
                    },
                    ...
                ]
            }

            Here I gave an example of one character but there may be multiple characters in an array so I put "..." to signify that.  You may not include "..." in your response.  You must completely fill in the arrays.   The <character_name_string> is the name of the character that can be found in the room, <character_description_string> is a description of the character. The characterTalkingTopics contains all of the talking topics the player has to talk about with the charater to make progress in the game.  Every talking topic the character may have for this specific chapter, objective and when the character is in this room must be included in characterTalkingTopics.  Do not combine separate talking topics into one topic.  The characterTalkingTopics must correspond to the objective.  The prerequisites array is a list of prerequisites required to talk about a topic with the character.  The characterSays string is a what the actually character response is talks about topic.  For example, if a specific puzzle needs to be completed first, then it must be listed as a prerequisite.  Make sure you list prerequisites in the order they must be accomplished in the game.  Each character must be defined by a JSON5 object and must contain name, description, conditions, and characterTalkingTopics. You cannot leave out any of these properties.  The characterTalkingTopics JSON5 objects (if they exist), must contain topic, prerequisites, gameWorldStateChange, and pointsGains.   You cannot leave out any of these properties.   You must include *all* of the characters that appear this room.  You may also, very rarely, include some unnecessary characters to make the game more challenging.
            */

            // kludge
            if (roomCharactersJson == null || !roomCharactersJson.characters || !isIterable(roomCharactersJson.characters) || roomCharactersJson.characters.length==0)
                roomCharactersJson = await repeatCommand();


            room.characters = roomCharactersJson.characters

            await writeOutGameJson(myGameJSON)
        }
    }
}

/*
stopCommand()
*/
