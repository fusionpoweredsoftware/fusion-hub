//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
//const actor = await reloadModule("fusion-hub/app-lib/conversation-gen");

await actor.process({
    getUnprocessedActorResponse: async ({ person1, person2, biosignature, rules, explanation, input }) => {
        /*(actorData,UIDMD:20240128T061128170Z,-person,-biosignature,-rules,input) chat-with-ai I have provided you with the person, biosignature, rules, and input code blocks using UIDMD notation. You are to provide the response code block using UIDMD notation, as if you were ${person1} having a conversation with ${person2}.${explanation}*/;
        return actorData;
    },
    ai, app, input, output
});

/*STOP*/
/*start app-using-ai*/