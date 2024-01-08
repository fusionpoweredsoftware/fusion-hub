function aiInstructionsForFilenameCreation(fileExtension="JSON",charactersMax=32) {
    return `Come up with a .${fileExtension} filename to describe the nature of this conversation using less than ${charactersMax} characters in double quotes (")`
}

let defaultAiInstructionsForConversationStart = `Only speak in English unless instructed otherwise. Try your best to be a helpful assistant.`;

export default {
    aiInstructionsForFilenameCreation,
    defaultAiInstructionsForConversationStart
}