export function translateToVertexHistory(computatronHistory) {
    return computatronHistory.map(x=>{ 
      x.role=(x.role=='assistant' ? 'model' : x.role); 
      x.parts = [{ "text": x.content }]; 
      delete x.content;
      return x
    });
}

export function translateFromVertexHistory(vertexHistory) {
    return vertexHistory.map(x=>{ 
      x.role=(x.role=='model' ? 'assistant' : x.role);
      if (x.parts) {
        x.content = x.parts[0].text; 
        delete x.parts;
      }
      return x
    });
}