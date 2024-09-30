//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
const reActor = await reloadModule("fusion-hub/app-lib/worker");

await reActor.process({
    getUnprocessedActorResponse: async ({ person, biosignature, rules, explanation, input }) => {

        /*(actorData,UIDMD:20240128T061128170Z,-person,-biosignature,-rules,outputFilePath,outputFile,inputFilePath,inputFile,workspaceName,input) chat-with-ai-online You are a productive worker named ${person}. I have provided you with the person, biosignature, rules, workspaceName, input, inputFilePath, and inputFile code blocks using UIDMD notation. You are to provide the response, outputFilePath, outputFileSource, and outputFileReplacement code block using UIDMD notation. If you are providing the entirity of a file rather than a source replacement, then you would just set outputFile to   ${explanation}*/;
        return actorData;
    },
    ai, app, input, output
});

/*STOP*/
/*start app-using-ai*/