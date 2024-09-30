//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:

const otfa = await reloadModule("fusion-hub/app-lib/otfa-text");

await otfa.process({
    getUnprocessedOtfaResponse: async ({input, explanation="", type}) => {
        /*(res,text) chat-with-ai-online Respond like I'm playing a ${type}text adventure. Award no points at the beginning of the game. Do not provide the possible choices or any directed decisions for the player. Rather, let the player explore freely. The very beginning of your response must match the regex form /^You/g. NOTE: ^ should not be part of your response; it\'s used to indicate the start of the response for the regex engine.. My command is: ${input}${explanation}*/
        return res;
    },
    getUnprocessedInvResponse: async ({inventory, clarify}) => {
        /*(res,text) chat-with-ai-online Verify the inventory contents by providing the inventory in JS array string form ["item1","item2","etc."]. This means that your response can ONLY be of the form /\[("[a-z0-9 ]+")*\]/ig (it should be the same as last known state UNLESS items were dropped or picked up). This is the last known inventory state: ${inventory}*/
        return res;
    },
    getUnprocessedResponseVerification: async () => {
        /*(res,text) chat-with-ai-online Are you giving the player options of what they can do? Just say Yes or No with no decoration. Only /^(yes|no)$/ig will match this response.*/;
        return res
    },
    askQuestion: async (question, responseFormRestriction='',flags='g') => {
        let regex;
        if (responseFormRestriction)
            regex = new RegExp(responseFormRestriction,flags);
        /*(res,text) chat-with-ai-online ${question}${responseFormRestriction ? ('/'+responseFormRestriction+'/' + flags) : ''}*/
        if (responseFormRestriction && !regex.test(res)) {
            /*(res2,text) chat-with-ai-online Your answer MUST meet the response form restriction: ${'/'+responseFormRestriction+'/' + flags}\n\nSo let's try that again, and this time make sure you follow the response restrictions: ${question}*/
            res=null;
            if (responseFormRestriction && !regex.test(res2)) {
                /*(res3,text) chat-with-ai-online It seems you're having difficulty following my instructions. Let's try to make this easier. Rather than trying to match an exact form, I will let you anchor where the form-restricted response starts and ends using ^ and $, where ^ indicates the start and $ indicates the end. For example, you "A simple response." would be " ^A simple response.$ " \n\nIf that makes sense, please use that form to answer the question: ${question}*/
                res2=null;
                const regex = /^\^(.*?)\$$/;
                res3 = res3.match(regex);
                if (res3=='A simple response.' || regex.test(res3)) {
                    res3 = null;
                }
                ai.chatHistory[ai.channel].pop();
                ai.chatHistory[ai.channel].pop();
                ai.chatHistory[ai.channel].pop();
                ai.chatHistory[ai.channel].pop();
            } else {
                ai.chatHistory[ai.channel].pop();
                ai.chatHistory[ai.channel].pop();
            }
        }
        ai.chatHistory[ai.channel].pop();
        ai.chatHistory[ai.channel].pop();
        return res || res2 || res3;
    },
    ai, app, input, output
});

/*STOP*/