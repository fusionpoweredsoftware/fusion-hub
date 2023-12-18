import cli from 'fusion-hub/cli';
export const settings = {
    name: process.env.FUSION_HUB_INSTANCE_NAME || 'Fusion Hub',
    port: process.env.FUSION_HUB_INSTANCE_PORT || 3000,
    preconfigure: async (fusionHub, preconfigurationSettings) => {
        await fusionHub.executeCommand('chat-with-ai', { isNew: true, ...preconfigurationSettings });
    },
    handleListeningStarted: (webApiName, webApiPort) => {
       cli.output.log(`${webApiName} is listening on port ${webApiPort}`);
    },
    asyncRequestLimit: 2,
    waitBetweenRequestsMS: 2000,
    asyncRequestTimeoutMS: 2 * 60 * 1000
}
export default {
    routes: {
        'chat-with-ai': {
            preRequest: (route) => cli.output.log(`Listening for /${route} requests...`),
            request: (request, defaultRequestSettings) => [{
                executeCommand: `chat-with-ai ${request.message}`,
                executeCommandSettings: {aiChatSettings: defaultRequestSettings, initParameters: defaultRequestSettings}
            }],
            preResponse: (responseWrite, request) => ((res, { sessionId, responseId }) => responseWrite(res, {request: request.message, responseId, sessionId})),
            response: (responseWrite) => ((res, { sessionId, responseId, message }) => responseWrite(res, {message, responseId, sessionId})),
            postResponse: (responseWrite) => ((res, { sessionId, responseId }) => responseWrite(res, {state: 'eom', responseId, sessionId}))
        }
    }
}


