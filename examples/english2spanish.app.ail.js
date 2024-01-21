//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP
//script:

/*start chat-with-ai
(translationData, JSON)
This is not a conversation. I only want you to translate this text in quotes to Spanish: 
"${app.input}"

Please reply in JSON format. For example: 
{
    english: "<words in quote>",
    translationToSpanish: "<translate english into spanish>",
    isSpanishToEnglishTranslation: <boolean confirming that the english was translated into spanish>
}

So, for example, if a user says "hi" you would return:
{
    english: "hi",
    translationToSpanish: "hola",
    isSpanishToEnglishTranslation: true
}
*/
let attempts=0;
const isInvalid = () => (translationData==null || !isString(translationData.translationToSpanish) || translationData.isSpanishToEnglishTranslation);
while (isInvalid() && attempts++<5)
    translationData=await repeatCommand();

if (isInvalid())
    print("Unable to translate.");
else
    print(translationData.translationToSpanish);