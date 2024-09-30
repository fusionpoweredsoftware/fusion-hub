//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
const otfa = await reloadModule("fusion-hub/app-lib/otfa");

await otfa.process({
    getUnprocessedOtfaResponse: async ({input, explanation=""}) => {
        let type = '';
        if (app.settings.theme)
            type=`${app.settings.theme} themed `
        /*(response,UIDMD:20240128T061128170Z,-textAdventureGameResponse,-pointsAcquired,command) chat-with-ai-online Respond in UIDMD notation like I'm playing a ${type}text adventure. Do not provide the possible choices for the player. Rather, let the player explore freely. My command is: ${input}${explanation}*/;
        return response;
    },
    ai, app, input, output
});

/*STOP*/
/*start app-using-ai*/