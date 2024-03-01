import { reloadModule } from 'fusion-hub/functions';
import { isObject, isString } from 'fusion-hub/functions';

export async function process(data) {
    let { input, output, app, ai, getUnprocessedActorResponse } = data;
    let conversation = "Today we are discussing: " + input;
    let initialTopic = input;
    let i=0;
    let reActorAIs = [];
    const actor = await reloadModule("fusion-hub/app-lib/actor");
    app.settings.conversationLength||=1;
    while ((app.settings.conversationLength--)>0) {
        app.settings.currentId = 0;
        while (app.settings.currentId<app.settings.ids.length) {
            if ((app.settings.currentId)>=reActorAIs.length) {
                reActorAIs.push([]);
            }
            delete app.settings.person;
            delete app.settings.biosignature;
            app.settings.id=app.settings.ids[app.settings.currentId];
            
            app.settings.currentId++;
            const finalResponse = await actor.process({
                getUnprocessedActorResponse,
                ai, app, input, output
            });
            if (isObject(finalResponse) && isString(finalResponse.value)) {
                reActorAIs.push()
                ai.chatHistory[0].pop ();
                ai.chatHistory[0].pop ();
                ai.chatHistory[0].splice(4,2, {
                    "role": "user",
                    "parts": [{
                        "text": "Avoid repeating the same initial words or phrases in consecutive sentences or responses to promote variety and maintain the reader's interest. This is the conversation so far:\n\n"+conversation
                    }]
                }, {
                    "role": "model",
                    "parts": [{
                        "text": `Okay. I'll keep that in mind when come up with each person's responses. I will avoid repeating the same initial words or phrases in consecutive sentences or responses to promote variety and maintain the reader's interest.`
                    }]
                });
                /*if (i%2==0) {
                    conversation = "Today we are discussing: " + initialTopic;
                }*/
                conversation += `\n\n${app.settings.person} said: `+finalResponse.value
                /*if (((i++)%5)==0)
                    input = `Ask ${app.settings.person} a one or two sentence question. Do not use commas in your first sentence. Limit question to 2 sentences. Remember the topic is: ${initialTopic}`;
                else
                    input = `Answer ${app.settings.person} any unresolved questions using unique words that haven't been said before. Limit responses to 1 to 3 sentences. Do not use commas in your first sentence. Remember the topic is: ${initialTopic}`;
                if (((i++)%7)==0) {
                    conversation = "Today we are discussing: " + initialTopic;
                    input = `Answer whatever question ${app.settings.person} might have (if any) and change the sub-topic. Limit responses to 1 to 3 sentences. Do not use commas in your first sentence.`;
                }*/
                if (app.settings.person=="VIDEO")
                    input = "Give VIDEO a programming task to complete."
            }
        }
    }
}