import {
    getHistory,
    sendMessage,
    setHistory
} from './api.mjs';
import {
    translateToVertexHistory,
    translateFromVertexHistory
} from './translator.mjs';
import fs from 'fs/promises';
import _fs from 'fs';

const queue = {
    items: [],
    waiting: null,

    add(item) {
        if (this.waiting) {
            this.waiting(item); // Resolve the pending promise with the new item
            this.waiting = null;
        } else {
            this.items.push(item); // Queue the item if there's no waiting consumer
        }
    },

    async get() {
        if (this.items.length > 0) {
            return this.items.shift(); // Return an item if available
        }

        // Otherwise, wait for a new item
        return new Promise(resolve => {
            this.waiting = resolve;
        });
    }
};

export async function emulateVertexGenerativeModel(environment, sessionId, routeFileName, port, host) {
    await fs.writeFile(routeFileName, `["/"]`);
    let resolver;
    const emulation = {
        historyInternal: []
    }
    const custom_console = {
            error: async (err) => {
                throw err;
            }
        },
        stdout = {
            write: async (writeData) => {
                const item = {
                    candidates: [{
                        content: {
                            parts: [
                                { text: writeData }
                            ]
                        }
                    }]
                }
                queue.add(item);
            }
        };
        
    custom_console.log = async (logData) => {
        if (logData.log_class == 'full_response') {
            resolver(logData.filtered_response);
            queue.add(null);
        }
        if (logData.log_class == "continuation_route") {
            emulation.historyInternal.length = 0;
            const computatronHistory = await getHistory(environment, logData.continuation_route, port, host, custom_console)
            const history = translateToVertexHistory(computatronHistory)
            Object.assign(emulation.historyInternal, history);
        }
    }
    emulation.sendMessageStream = async (textObjectArray) => {
        const { text: message } = textObjectArray[0];
        const computatronHistory = translateFromVertexHistory(emulation.historyInternal);
        const continuation_route = await setHistory(environment, computatronHistory, port, host, custom_console);
        if (!_fs.existsSync(routeFileName))
            await fs.writeFile(routeFileName, `["/"]`);
        let routeRaw = await fs.readFile(routeFileName, 'utf-8');
        let routeArray = JSON.parse(routeRaw);
        routeArray[sessionId] = continuation_route;
        await fs.writeFile(routeFileName, JSON.stringify(routeArray));
        await sendMessage(environment, message, routeFileName, port, host, sessionId, custom_console, stdout);
        const stream = {
            [Symbol.asyncIterator]: async function* () {
                while (true) {
                    const item = await queue.get(); // Wait until a new item is available in the queue
                    if (item === null) {
                        break; // Exit the loop if a 'null' signal is received
                    }
                    yield item; // Yield the item to the consumer
                }
            }
        };
        const response = new Promise((resolve) => {
            resolver = resolve;
        });
        return {
            stream,
            response
        }
    }
    return {
        startChat: async () => {
            emulation.historyInternal.length=0;
            await fs.writeFile(routeFileName, `["/"]`);
            return emulation
        }
    }
}