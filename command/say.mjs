import play from 'play-sound';
import { output } from "fusion-hub/cli";
import { expandFilePath } from 'fusion-hub/functions'
import textToSpeech from '@google-cloud/text-to-speech';
import crypto from 'crypto';
import _fs from 'fs';
import fs from 'fs';
import util from 'util';

export const command = {
  id: "say",
  name: "say",
  description: "Converts text to speech.",
  examples: `say sync: (male-voice) hello there world!`,
  isInputCaseSensitive: false,
  isStartable: false,
};

function md5(data) {
  return crypto.createHash('md5').update(data).digest('hex');
}

const client = new textToSpeech.TextToSpeechClient();
const player = play({});

function playSound(outputFile) {
    return new Promise((resolve, reject) => {
        player.play(outputFile, (err) => {
            if (err) {
                reject(err);
            } else {
                resolve();
            }
        });
    });
}
 
async function synthesizeSpeech(text, name, isSync=false) {

  const outputFile = await expandFilePath(`~~~/output-${md5(text)}-${name}.mp3`, { contextDir: "tts"});
  const createFile = async () => {
    if (!_fs.existsSync(outputFile)) {
      const input = { text };
      const voice = { languageCode: 'en-US', name };
      const audioConfig = { audioEncoding: 'MP3', speakingRate: 1 };

      const request = {
          input: input,
          voice: voice,
          audioConfig: audioConfig,
      };

      const [response] = await client.synthesizeSpeech(request);
      const writeFile = util.promisify(fs.writeFile);
      await writeFile(outputFile, response.audioContent, 'binary');
    }
  }
  await createFile();
  if ((_fs.statSync(outputFile)) < 1024)
    await createFile();
  if (isSync) {
    await playSound(outputFile);
  } else {
    playSound(outputFile);
  }
}

export async function process({ joinedInput, settings }) {
  const { error } = output;
  let cleanInput = joinedInput;
  let { gender = 'F', isSync = false } = settings;
  if (cleanInput.startsWith("sync: ")) {
    isSync = true;
    cleanInput=joinedInput.replace(/^sync\: /g,'');
  }
  if (cleanInput.startsWith("(male-voice) ")) {
    gender = 'D';
    cleanInput=joinedInput.replace(/^\(male\-voice\) /g,'');
  }
  await synthesizeSpeech(cleanInput, `en-US-Journey-${gender}`, isSync).catch(error);  
}
