import { expandFilePath, extractLambdaFunctionBody, filterOutput, isObject, isString, stabilizeOutput, addPeriodToBasename, getUidMdExplanation, generateUUID, reloadModule } from 'fusion-hub/functions';
import cli, { properties as cliProperties, executeCommand } from 'fusion-hub/cli';
import path from 'path';
import fs from 'fs/promises';
import _fs from 'fs';
import crypto from 'crypto';
import json5 from 'json5';
import setup from 'fusion-hub/setup'

export const ai = {
    chatHistory: { 0: [] },
    channel: 0
}

export const parser = {
    json5,
    stabilizeOutput
}

export const lastSetup = {};

let lastCommand = null;
let lastStopCommand = null;
let lastVarType = null;
let lastOutput = null;
let lastSettings = null;

export const escapedCommand = {
    id: "multi-command",
    name: "multi-command",
    description: "Run multiple commands from a file.",
    examples: `multi-command <file.txt> [parameters]`,
    isInputCaseSensitive: true,
    isStartable: false
  };

export async function process({input, joinedInput, settings = {}}) {
    const { error, log } = cli.output;
    const filePath = input[0];
    if (input.length < 1 || filePath == '')
    {
        log(escapedCommand.examples);
        return;
    }
    settings.aiChatSettings ||= {};
    settings.aiChatSettings.chatHistory ||= { 0: [] };
    settings.app ||= {};
    settings.app.input ||= '';
    settings.app.settings ||= {};
    if (settings.app.input)
        input = settings.app.input;
    const { mcFileContextDir = 'multi-command', historyContextDir = 'multi-command-history'  } = settings;
    const { isLoggingMultiCommandFileContents = true, isVerbose = true, isInterruptable = true, app = {}, aiChatSettings = {}} = settings;
    if (aiChatSettings.chatHistory && ai.chatHistory != aiChatSettings.chatHistory) {
        ai.chatHistory[0].length=0;
        ai.chatHistory[0].push(...aiChatSettings.chatHistory[0]); 
    }
    aiChatSettings.chatHistory = ai.chatHistory;
    aiChatSettings.channel = ai.channel;
    let readFilePath = await expandFilePath(filePath, {contextDir: mcFileContextDir})
    let commandText = await fs.readFile(readFilePath, 'utf-8');
    commandText = commandText.replace(/^[^\n]+\n/, 'multi-command ' + joinedInput + '\n');
    let hashCommandText = commandText.replace(/multi-command [^ ]+/, '');
    const hash = crypto.createHash('sha256').update(hashCommandText).digest('hex');

    let historyFilename = path.basename(filePath);
    if (/-[0-9a-f]{64}/.test(historyFilename))
    {
        historyFilename = historyFilename.replace(/-[^ ]{64}$/, '-' + hash);
    }
    else
    {
        historyFilename += hash;
    }

    const historyFileShortPath = path.join('~~~', historyFilename);
    const historyFile = await expandFilePath(historyFileShortPath, {contextDir: historyContextDir});
    let historyExists = _fs.existsSync(historyFile);
    
    if (!historyExists)
    {
        await fs.writeFile(historyFile, commandText);
        if (isVerbose)
            log(`Creating history file as: ${historyFileShortPath}`);
    }
    else
    {
        if (isVerbose)
            log(`History file ${historyFile} already exists.`);
    }

    let lines = commandText.split(/\r\n|\n/);
    const stopCommand = lines[3].slice(2);
    let scriptStart=3;
    for (; scriptStart<lines.length; scriptStart++)
    {
        if (lines[scriptStart].slice(2)=='script:')
            break;
    }
    let script={};
    if (lines.length > scriptStart)
    {
        let codeToEval= '\n' + lines.slice(scriptStart+1).join('\n');

        let codeToEvalSplit = codeToEval.split(new RegExp(lines[1].slice(2)));
        let codeToEvalFinal = codeToEvalSplit[0]+'\n';
        let commandBlockClosed = true; 
        Object.assign(lastSetup, {
            scriptVariables: script,
            commandBlockClosed
        });
        
        let commandAndCode = null
        for (let i=1; i < codeToEvalSplit.length; i++)
        {
            commandAndCode = codeToEvalSplit[i].split(new RegExp(lines[2].slice(2)))

            if (commandAndCode.length >= 1 && commandAndCode.length <= 2)
            {
                let prependAssignment = '';
                let varType = 'null';
                let varTypeSettings = 'null';
                let mcVarRegex = /^\([ ]*(?<varName>[_a-zA-Z0-9]+)[ ]*,[ ]*(?<varType>[_a-zA-Z0-9]+)[ ]*(:[ ]*(?<varTypeSettings>[-+_a-zA-Z0-9,?]+)[ ]*)?\)\n?/m;
                let matches = commandAndCode[0].match(mcVarRegex);
                if (matches != null)
                {
                    prependAssignment = matches.groups.varName ? ('let ' + matches.groups.varName + '=') : null;
                    varType = matches.groups.varType || null;
                    varTypeSettings = matches.groups.varTypeSettings || null;
                
                    if (prependAssignment && varType && /[a-zA-Z]{4,}/g.test(varType))
                    {
                        commandAndCode[0] = commandAndCode[0].replace(mcVarRegex, '')
                    }
                    else
                    {
                        error('Invalid command assignment: (' + matches.groups.varName + ', ' + varType + ')');
                    }
                }
                let escapedCommand = commandAndCode[0].replace(/`/g, '\\`')
                if (varType=='UIDMD' && varTypeSettings) {
                    let uidMdInput = varTypeSettings.split(",");
                    let uuid = uidMdInput[0];
                    uidMdInput=uidMdInput.slice(1);
                    if (uidMdInput.length>0) {
                        escapedCommand+=`\n\nProvided code blocks:`;
                        for (let i = 0; i < uidMdInput.length; i++) {
                            uidMdInput[i]=uidMdInput[i].replace(/[-+?]/g,'');
                            escapedCommand+=`\n\n***plaintext (${uuid}:${uidMdInput[i]})\n\$\{${uidMdInput[i]}\}\n(${uuid}:${uidMdInput[i]})`;
                        }
                    }
                }
                const funcBodyToEval = async () => (await evalCommand(`${escapedCommand}`, { aiChatSettings, stopCommandIn: `${stopCommand}` , varTypeIn: `${varType}`, varTypeSettings: `${varTypeSettings}`, cliProperties}));
                const codeBody = extractLambdaFunctionBody(funcBodyToEval, { varType, escapedCommand, stopCommand, varTypeSettings });
                codeToEvalFinal += `\n${prependAssignment}${codeBody};\n`;
                codeToEvalFinal += (commandAndCode.length == 2) ? commandAndCode[1]: '';
            }
            else if (commandAndCode.length > 2)
            {
                error('Invalid syntax: Double terminator detected.');
            }
        }

        if (commandAndCode && commandAndCode.length != 2)
        {
            error('Open command without close.');
        }
        const mcRunner = await fs.readFile('./mc-runner.mjs','utf-8');
        codeToEvalFinal = mcRunner.replace("/*${codeToEvalFinal}*/",codeToEvalFinal);
        let debuggableFile = addPeriodToBasename(readFilePath)
        debuggableFile = `${debuggableFile}.mjs`.replace(/\.js\.mjs$/g,'.mjs');
        if (isLoggingMultiCommandFileContents)
            log(codeToEvalFinal)
        await fs.writeFile(debuggableFile,codeToEvalFinal);
        const importedCode = await reloadModule(debuggableFile, {isExtensionIncluded: true});
        if (isInterruptable) {
            importedCode.run(input, app, aiChatSettings);
        } else
            await importedCode.run(input, app, aiChatSettings);
    }
}

export async function repeatCommand(additionalInfo)
{
    return await evalCommand(null, { repeatQuery: true, additionalInfo});
}

export async function evalCommand(commandIn, settings)
{
    const { error } = cli.output;
    if (typeof settings !== 'object' || settings === null) {
      error ("The evalCommand requires settings to be an object.");
      return;
    }
  
    settings ||= {};
  
    settings.stopCommandIn ||= null;
    settings.varTypeIn ||= null;
    settings.repeatQuery ||= false;
    settings.aiChatSettings ||= {};
    
    const { repeatQuery, varTypeSettings } = settings;
    let output = null;

    let command = commandIn;
    let { stopCommandIn: stopCommand, varTypeIn: varType, additionalInfo } = settings;

    if (additionalInfo)
        additionalInfo+=" ";

    if (!repeatQuery)
    {
        lastCommand = command;
        lastStopCommand = stopCommand;
        lastVarType = varType;
        lastSettings = settings;
    }
    else
    {
        command = lastCommand;
        stopCommand = lastStopCommand;
        varType = lastVarType;
        settings = lastSettings;
        if (varType=='JSON')
            command += "  Please double check your JSON.  Your last response contained malformed JSON.";
    }

    const { isCommandStartLogged = false, aiChatSettings = {} } = settings;

    Object.assign(aiChatSettings, settings.aiChatSettings);
    aiChatSettings.isStreaming ||= false;

    if (command == stopCommand)
    {
        cliProperties.commandMode = null;
    }
    else 
    {   
        try {
            if (varType=="UIDMD") {
                const uuid = generateUUID();
                if (ai.chatHistory[0].length>2) {
                    ai.chatHistory[0].shift();
                    ai.chatHistory[0].shift();
                }
                if (varTypeSettings) {
                    let uidMdInput = varTypeSettings.split(",");
                    command=command.replace(new RegExp(uidMdInput[0],"g"),uuid);
                    let cleanChatHistory = JSON.parse(JSON.stringify(ai.chatHistory[0]));
                    if (cleanChatHistory.length>4) {
                        for ( let i = 0; i < 4; i++)
                            cleanChatHistory.shift();
                        cleanChatHistory=JSON.stringify(cleanChatHistory);
                        for (let i = 0; i < uidMdInput.length; i++) {
                            if (uidMdInput[i][0]=='-') {
                                uidMdInput[i]=uidMdInput[i].replace(/[-+?]/g,'');
                                cleanChatHistory = cleanChatHistory.replace(new RegExp(`\\\\n\\\\n\\*\\*\\*[_a-zA-Z0-9]* \\(([_a-zA-Z0-9]*):${uidMdInput[i]}\\)\\\\n.*\\\\n\\(\\1:${uidMdInput[i]}\\)`,`g`),'');
                            }
                        }
                        cleanChatHistory=JSON.parse(cleanChatHistory);
                        ai.chatHistory[0].length-4;
                        while (ai.chatHistory[0].length>4)
                            ai.chatHistory[0].pop();
                        ai.chatHistory[0].push(...cleanChatHistory);
                    }
                }
                ai.chatHistory[0].unshift({
                    "role": "user",
                    "parts": [{
                        "text": getUidMdExplanation(uuid)
                    }]
                }, {
                    "role": "model",
                    "parts": [{
                        "text": `That's an interesting notation! I will be sure to use UIDMD notation for each code block, whenever you ask to provide it. For example, if I have a plain-text code block about whales I will say: 
                    
                        ***plaintext (${uuid}:whales)
                        Did you know baby orcas are called calves?
                        (${uuid}:whales)
                        \"\n    }`
                    }]
                });
            }
            const finalCommand = `${command.replace(/`/g, '\\`')}${(repeatQuery && isString(lastOutput) && varType=='JSON') ? `\n\nUse the output from the JSON parser to help resolve the issue: ${lastOutput}` : ''} ${additionalInfo||''}`;
            output = await executeCommand(finalCommand, { isThrowing: true, aiChatSettings, isCommandStartLogged  });
            if (varType!='null' && isString(output))
            {
                output = await filterOutput(output, {typeOfVar: varType, varTypeSettings});
            }
            if (!repeatQuery)
                lastOutput=output;
            /// kludge
            const isErrorMessage = isString(output);
            if ((!isObject(output) || isErrorMessage) && varType=='JSON')
            {
                output = await executeCommand(await eval(`(async () => \`${command.replace(/`/g, '\\`')}\nDo not leave any of the JSON out.  You must produce valid JSON.  Do not produce the same response as last time!!! ${isErrorMessage ? `\n\nUse the output from the JSON parser to help resolve the issue: ${output}` : ''}\`)()`), { isThrowing: true, aiChatSettings, isCommandStartLogged  });
                if (varType && output)
                {
                    output = await filterOutput(output, {typeOfVar: varType, varTypeSettings});
                }
            }
        } catch (err)
        {
            output = null;
            error(err + '\nAdditional information:\n\nFailed on sub-command:\n' + command);
        }
    }

    return output;
}
