// Static hosts may serve .gz verbatim or decode it through Content-Encoding.
// Detect the bytes so both modes work without a server configuration change.
export async function decodeWasm(input) {
  const bytes=new Uint8Array(input.buffer ?? input,input.byteOffset ?? 0,input.byteLength);
  if(bytes[0]===0 && bytes[1]===97 && bytes[2]===115 && bytes[3]===109) return bytes;
  if(bytes[0]===31 && bytes[1]===139 && typeof DecompressionStream==='function') {
    const result=new Uint8Array(await new Response(new Blob([bytes]).stream().pipeThrough(new DecompressionStream('gzip'))).arrayBuffer());
    if(result[0]===0 && result[1]===97 && result[2]===115 && result[3]===109) return result;
  }
  throw new Error('下载内容不是有效的 WASM 模块');
}
