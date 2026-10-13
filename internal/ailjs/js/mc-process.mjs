// The preprocessing half of the original command/multi-command.mjs (its
// `process` function), kept apart from the verbatim copy in legacy/ because the
// embedded JS engine has no named capture groups. The only edits:
//   - mcVarRegex uses numbered groups (1 = varName, 2 = varType, 4 = varTypeSettings)
//   - run() is always awaited, so the REPL prompt comes back when the app is done
//   - the evalCommand(...) line for each block is built as a string instead of
//     being read back from an arrow function with Function.prototype.toString,
//     which the engine gets wrong by one character
import { expandFilePath, addPeriodToBasename, reloadModule } from 'fusion-hub/functions';
import cli from 'fusion-hub/cli';
import path from 'path';
import fs from 'fs/promises';
import _fs from 'fs';
import crypto from 'crypto';
import { ai, lastSetup, escapedCommand } from 'fusion-hub/command/multi-command';

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
                let mcVarRegex = /^\([ ]*([_a-zA-Z0-9]+)[ ]*,[ ]*([_a-zA-Z0-9]+)[ ]*(:[ ]*([-+_a-zA-Z0-9,?]+)[ ]*)?\)\n?/m;
                let matches = commandAndCode[0].match(mcVarRegex);
                if (matches != null)
                {
                    prependAssignment = matches[1] ? ('let ' + matches[1] + '=') : null;
                    varType = matches[2] || null;
                    varTypeSettings = matches[4] || null;
                
                    if (prependAssignment && varType && /[a-zA-Z]{4,}/g.test(varType))
                    {
                        commandAndCode[0] = commandAndCode[0].replace(mcVarRegex, '')
                    }
                    else
                    {
                        error('Invalid command assignment: (' + matches[1] + ', ' + varType + ')');
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
                const codeBody = ('await evalCommand(`' + escapedCommand + '`, { aiChatSettings, stopCommandIn: `' + stopCommand + '` , varTypeIn: `' + varType + '`, varTypeSettings: `' + varTypeSettings + '`, cliProperties})').replace(/\`null\`/g, 'null');
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
        await importedCode.run(input, app, aiChatSettings);
    }
}
