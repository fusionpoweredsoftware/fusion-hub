import axios from 'axios';
import fs from 'fs/promises';
import _fs from 'fs';
import _ from 'lodash';

export async function setHistory(environment, history, port = 3000, host = "http://localhost",custom_console=console) {
    const URL = host + ":" + port + `/set-continuation-route/${environment}`;
    let route;
    await axios.post(URL, {
        history
    }, {
        responseType: 'json'
    })
    .then(response => {
        route = response.data.continuation_route;
    })
    .catch(async (error) => {
        await custom_console.error(error);
    });
    return route;
}

export async function getHistory(environment, route, port = 3000, host = "http://localhost",custom_console=console) {
    const URL = host + ":" + port + `/message-history/${environment}`;
    let history = ''
    await axios.post(URL, {
        route
    }, {
        responseType: 'json'
    })
    .then(response => {
        history = response.data;
    })
    .catch(async (error) => {
        await custom_console.error(error);
    });
    return history;
}

export async function sendMessage(environment, message, routeFileName = "session_routes.json", port = 3000, host = "http://localhost", sessionId = 0, custom_console=console, stdout = process.stdout) {
    const URL1 = host + ":" + port + `/message/${environment}`;
    const URL2 = host + ":" + port + `/get-continuation-route/json/${environment}`;

    if (!_fs.existsSync(routeFileName))
        await fs.writeFile(routeFileName, `["/"]`);
    let routeRaw = (routeFileName) ? (await fs.readFile(routeFileName, 'utf-8')) : `["/"]`;
    let routeArray = JSON.parse(routeRaw);
    let route = routeArray[sessionId];

    async function getContinuationRoute(uid) {
        try {
            const response = await axios.post(URL2, uid, {
                responseType: 'json'
            });
            return response.data;
        } catch (error) {
            await custom_console.error(error);
            throw error;
        }
    }

    function getMatch(stream) {
        return stream.match(/^\n"UID":"([a-z0-9]{64})"\n\n/g);
    }

    async function streamResponse(responseRoute) {
        await axios.post(URL1, {
                route: responseRoute
            }, {
                responseType: 'stream'
            })
            .then(response => {
                let filtered_response = '';
                let raw_response = '';
                let previousData = '';
                let _match;
                response.data.on('data', async (chunk) => {
                    const newData = chunk.toString().replace(previousData, '');
                    if (newData !== '') {
                        raw_response += newData;
                        if (_match && _match[0]) {
                            await stdout.write(newData);
                            filtered_response += newData;
                        }
                        _match = getMatch(raw_response);
                    }
                    previousData = chunk.toString();
                });
                response.data.on('end', async () => {
                    await custom_console.log({ raw_response, filtered_response, log_class: "full_response" });
                    const match = getMatch(raw_response);
                    const uidJSON = JSON.parse(`{${match[0]}}`);
                    const route = await getContinuationRoute(uidJSON);
                    if (_.isObject(route) && route.cont) {
                        const continuation_route = route.cont;
                        routeArray[sessionId] = continuation_route;
                        await fs.writeFile(routeFileName, JSON.stringify(routeArray));
                        
                        await custom_console.log({continuation_route, environment, log_class: "continuation_route"});
                    } else {
                        if (_.isObject(route))
                            await custom_console.error("No route.cont as expected:", JSON.parse(route));
                        else
                            await custom_console.error("Route not an object as expected:", route);
                    }
                });
                response.data.on('error', async (error) => {
                    await custom_console.error(error);
                });
            })
            .catch(async (error) => {
                await custom_console.error(error);
            });
    }

    axios.post(URL1, {
            message,
            route
        }, {
            responseType: 'stream'
        })
        .then(response => {
            let stream = '';
            let previousData = '';
            response.data.on('data', chunk => {
                const newData = chunk.toString().replace(previousData, '');
                if (newData !== '') {
                    stream += newData;
                    //stdout.write(newData);
                }
                previousData = chunk.toString();
            });
            response.data.on('end', async () => {
                const match = stream.match(/^\n"UID":"([a-z0-9]{64})"\n\n/g);
                const uidJSON = JSON.parse(`{${match[0]}}`);
                const route = await getContinuationRoute(uidJSON);
                if (_.isObject(route) && route.cont) {
                    // custom_console.log("\n\n"+route.cont);
                    await streamResponse(route.cont);
                } else {
                    if (_.isObject(route))
                        await custom_console.error("No route.cont as expected:", JSON.parse(route));
                    else
                        await custom_console.error("Route not an object as expected:", route);
                }
            });
            response.data.on('error', async (error) => {
                await  custom_console.error(error);
            });
        })
        .catch(async (error) => {
            await custom_console.error(error);
        });
}