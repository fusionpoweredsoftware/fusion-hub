    import json5 from 'json5'

    function extractJson5Objects(text) {
        let braceCount = 0;
        let startIndex = 0;
        const jsonObjects = [];

        for (let i = 0; i < text.length; i++) {
            if (text[i] === '{') {
                braceCount += 1;
                if (braceCount === 1) {
                    startIndex = i; // Mark the start of a JSON5 object
                }
            } else if (text[i] === '}') {
                braceCount -= 1;
                if (braceCount === 0) {
                    // End of a JSON5 object
                    try {
                        const jsonObj = json5.parse(text.substring(startIndex, i + 1));
                        jsonObjects.push(jsonObj);
                    } catch (e) {
                        // Ignore if the string is not a valid JSON5 object
                    }
                }
            }
        }

        // Return based on the number of valid JSON5 objects
        return jsonObjects.length === 1 ? jsonObjects[0] : jsonObjects;
    }
  
  export const filterOutput = (output, filterSettings) => {
    //{typeOfVar: 'JSON'});
    if (filterSettings.typeOfVar == 'JSON')
    {
      return extractJson5Objects(output);
    }
    return filteredOutput;
  }
  

  let output = `Your input string here { a: 3 } and test test test {b:3, c: 4}`

  console.log(filterOutput(output, {typeOfVar: 'JSON'}));