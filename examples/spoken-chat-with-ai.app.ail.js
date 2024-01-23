//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
/*(response,text)
chat-with-ai ${app.input}*/

const speakableResponse = response.replace(/[^a-zA-Z0-9:;&\.?!\(\)\[\]\"\'\{\}\/\-\n ]/g,'').split(/\n\n/g);

for (let i = 0; i < speakableResponse.length; i++) {
    print(speakableResponse[i]+"\n")
    /*say sync: ${speakableResponse[i]}*/
}
/*STOP*/
/*start app-using-ai*/