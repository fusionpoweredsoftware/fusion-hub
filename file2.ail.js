**The adventure game template**
(?:\r\n|\n)----(?:\r\n|\n)
(?:\r\n|\n)(?:\$\$\$\$)
stopCommand()
script:

let script={};

function storyJSONobjectFormat()
{
    let str = '{ title, startGameStory';

    let minNumChapters = 10;
    let maxNumChapters = 16;
    let randNumChapters = minNumChapters + Math.floor(Math.random() * (maxNumChapters - minNumChapters + 1));
    for (let i=0; i<randNumChapters; i++)
    {
        str += `, chapter_${i}`;
    }

    str += '}';
}

----
setup vertex-ai-project as myvertexai-410504
----
start new chat-with-ai game-design-conversation.json
$$$$

script.defaultJSON=`{"theme": "Back-to-the-future"}`
script.json=JSON.parse(input[1] || script.defaultJSON) 
console.log('hello world')

let N=10;

for (let i=0; i < 2; i++)
{

----
Make a game story outline ${script.json.theme} themed adventure game with ${N} chapters.
Each chapter must be composed of a series of evevts.$
----
Put the story into a JSON object of the form ${storyJSONobjectFormat()}, 
where each chapter_i is of the form { chapterTitle, shortChapterDescrition }. 
Make sure you do *ALL* of the chapters.
$$$$

for (i=0; i<$(n); i++)
{
    ----
    Put chapter ${i} into a JSON object of the form {chapterTitle, chapter, event_1, event_2, event_3,..., event_K }, where K is the number of events in the chapter in the outline you wrote.  Do not leave out any events.
    $$$$
}

----
Create an array of JSON objects with details of each room in the form { name, description } for event1
Now provide an array of unique items in the form { name, description, command, combination } where command
is the text adventure command that the player has to perform with the item to make progress with it. Alternatively,
you can create a combination array which would be the array of items that are combined to make this item. If no
combination was required, simply leave the array empty.

IMPORTANT: Make sure to list every combination item in the inventory list and ONLY USE ENGLISH.
$$$$
}

----
stopCommand()
$$$$