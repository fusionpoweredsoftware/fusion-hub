//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP
//script:
/*start chat-with-ai
(translationData, JSON)
This is not a conversation. I just want you to translate this text in quotes to Spanish: 
"${app.input}"

Please reply in JSON format. For example: {
    english: "<words in quote>",
    translationToSpanish: "<translated english>" 
}
*/
if (translationData==null)
    print("Something went wrong. Try again later.");
else
    print(translationData.translationToSpanish)