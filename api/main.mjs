import fusionHub from 'fusion-hub';
import express from 'express';
import {
    generateUniqueId,
    responseWriteJSON,
    getChatStdout
} from 'fusion-hub/functions';
import config, {
    settings
} from 'fusion-hub/api/default-config';
const preconfigure = (preconfigurationSettings) => settings.preconfigure(fusionHub, preconfigurationSettings);
const app = express();
async function init() {
    const {
        name,
        port,
        handleListeningStarted,
        asyncRequestLimit,
        waitBetweenRequestsMS,
        asyncRequestTimeoutMS
    } = settings;
    app.use(express.json());
    let currentAsyncRequestCount = 0;
    for (let route in config.routes) {
        const { preRequest, request, preResponse, response, postResponse } = config.routes[route];
        app.post("/" + route + "/:sessionId?", async (req, res) => {
            const sessionId = req.params.sessionId || generateUniqueId();
            const responseId = generateUniqueId();
            const uidInfo = { sessionId, responseId };
            if (currentAsyncRequestCount > asyncRequestLimit) {
                responseWriteJSON(res, {
                    error: 429,
                    message: 'Too many requests.',
                    sessionId,
                    responseId
                })
                res.end('\n');
            }
            currentAsyncRequestCount++;
            let wasReduced = false;
            const reduceAsyncRequestCount = () => {
                if (!wasReduced) {
                    wasReduced = true;
                    currentAsyncRequestCount--;
                }
            }
            const timeoutId = setTimeout(reduceAsyncRequestCount, asyncRequestTimeoutMS);
            const requestData = req.body;
            res.setHeader('Content-Type', 'application/json');
            res.setHeader('Transfer-Encoding', 'chunked');
            preResponse(responseWriteJSON, requestData)(res, uidInfo);
            const chatStdout = getChatStdout(res, uidInfo, response(responseWriteJSON));
            const routeActions = request(requestData, { chatStdout, sessionId });
            for (let action of routeActions) {
                action.executeCommandSettings.apiCallSettings = {req, res: chatStdout, sessionId};
                await fusionHub.executeCommand(action.executeCommand, action.executeCommandSettings, action.executeCommandExtensiveInputBuffer);
            }
            
            postResponse(responseWriteJSON)(
                res, uidInfo);
            res.end('\n');
            clearInterval(timeoutId);
            setTimeout(reduceAsyncRequestCount, waitBetweenRequestsMS);
        });
        preRequest(route);
    }

    app.listen(port, () => handleListeningStarted(name, port));

}

export default {
    settings,
    config,
    app,
    preconfigure,
    init
}