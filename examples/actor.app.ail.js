//**Actor**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP
//script:

/*start chat-with-ai*/

async function getActor () {

    /*(actor,JSON)
    Provide the response of ${person} in the following form:
    {
        biosignature: "${biosignature}"
        person: "${person}",
        input: "${app.input}",
        response: "<put ${person}'s response here>"
    }

    Where input, provided as the known given variable in the JSON structure above, is what was said to the actor. */
    
    return actor;
}

let { person = "Albert Einstein" } = app.settings;
let { biosignature = null } = app.settings;

if (person === "Albert Einstein" && biosignature==null)
    biosignature="Hello I am Albert Einstein. I was born in March 14, 1879, and I conceived of the theory of special relativity and general relativity, which had a deep impact in science's understanding of physics.";

const actor = await getActor();
print("\n"+person+"> "+actor.response+"\n");

/*STOP*/
/*start app-using-ai*/