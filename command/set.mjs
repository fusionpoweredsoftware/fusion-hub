import setup from "fusion-hub/setup";
import { output } from "fusion-hub/cli";

export const command = {
  id: "set",
  name: "set",
  description: "Set a variables for commands to use, or see what is currently set.",
  examples: `set (<key> as <val>|json(+)? <json>|remove <key>|<initalize as null>)`,
  isInputCaseSensitive: true,
  isStartable: false,
};

export async function process({ input, joinedInput }) {
  const { print } = output;
  if (input.length == 1 && input[0] == 'json') {
    print(`JSON:\n\n${JSON.stringify(setup.settings)}\n`);
    return;
  }

  if (input[0]=="remove") {
    if (!input[1]) {
      print ("No variable specified.")
      return;
    }
    delete setup.settings[input[1]];
    print (`${input[1]} deleted.`)
    await setup.pushSetupSettings();
    return;
  }

  if (joinedInput.length == 0) {
    let simpleSettings = JSON.parse(JSON.stringify(setup.settings));
    print(JSON.stringify(simpleSettings, null, '\t'));
    return;
  }
  
  if (input.length == 2 && input[0] == 'json+') {
    Object.assign(setup.settings, JSON.parse(input[1]));
    await setup.pushSetupSettings();
    return;
  }
  else if (input.length == 2 && input[0] == 'json') {
    setup.settings = JSON.parse(input[1]);
    await setup.pushSetupSettings();
    return;
  }
  if (input.length==1) {
    Object.assign(
      setup.settings,
      { [input[0]]: null }
    );
    await setup.pushSetupSettings();
  } else if (input[1].toLowerCase()=="as") {
    input[1] = input[1].toLowerCase();
    for (let i = 0; i < input.length; i++) input[i] = `"${input[i]}"`;
    joinedInput = input.join(" ");

    const json = `{${joinedInput.replace(`"as"`, ":").replace(/' '/g, '","').replace(/," /g, '", ')}}`;
    Object.assign(
      setup.settings,
      JSON.parse(json)
    );
    await setup.pushSetupSettings();
  } else {
    print ("Syntax: <key> AS '<value>'")
  }
}
