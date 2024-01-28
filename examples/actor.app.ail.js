//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP
//script:

let { person = "Albert Einstein" } = app.settings;
let { biosignature = null } = app.settings;

if (person === "Albert Einstein" && biosignature==null)
    biosignature="Hello I am Albert Einstein. I was born in March 14, 1879, and I conceived of the theory of special relativity and general relativity, which had a deep impact in science's understanding of physics.";

/*(actor,UIDMD:20240128T061128170Z) chat-with-ai 
I have provided you with the person, biosignature, and input code blocks using UIDMD notation. You are to provide the response code block using UIDMD notation, as if you were ${person}.

Provided code blocks:

*** plaintext (20240128T061128170Z:person)
${person}
(20240128T061128170Z:person)

*** plaintext (20240128T061128170Z:biosignature)
${biosignature}
(20240128T061128170Z:biosignature)

*** plaintext (20240128T061128170Z:input)
${app.input}
(20240128T061128170Z:input)
*/

if (isObject(actor)) {
    const {response = {} } = actor;
    print("\nai> "+response.value);
} else {
    print("Bad response: "+response["*"])
    ai.chatHistory.pop();
    ai.chatHistory.pop();
}


/*STOP*/
/*start app-using-ai*/