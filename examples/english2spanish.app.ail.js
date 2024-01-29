//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP
//script:

/*start chat-with-ai*/

/*(translationData, UIDMD)
This is not a conversation. I only want you to translate this text in quotes to Spanish: 
"${app.input}"

Please reply in JSON format. For example: 
{
    "english": "<words in quote>",
    "translationToSpanish": "<translate english into spanish>",
    "isSpanishToEnglishTranslation": <boolean confirming that the english was translated into spanish>
}

So, for example, if a user says "Hi! I know spanish!" you would return:
{
    "english": "Hi! I know spanish!",
    "translationToSpanish": "¡Hola! ¡Sé español!",
    "isEnglishToSpanishTranslation": true
}
*/
let attempts=0;
const isInvalid = () => (translationData==null || !isString(translationData.translationToSpanish) || !translationData.isEnglishToSpanishTranslation);
while (isInvalid() && attempts++<5)
    translationData=await repeatCommand();

if (isInvalid())
    print("Unable to translate.");
else
    print(translationData.translationToSpanish);