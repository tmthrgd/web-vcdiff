import Module from '/vcddec.js';

async function moduleLoaded(m) {
	const dict = new Uint8Array([84, 104, 101, 32, 113, 117, 105, 99, 107, 32, 98, 114, 111, 119, 110, 32, 102, 111, 120, 32, 106, 117, 109, 112, 115, 32, 111, 118, 101, 114, 32, 116, 104, 101, 32, 108, 97, 122, 121, 32, 100, 111, 103, 46, 10]);
	const data = new Uint8Array([214, 195, 196, 0, 0, 1, 45, 0, 8, 44, 0, 0, 2, 1, 115, 44, 0]);

	const dictPtr = m._malloc(dict.length);
	const dataPtr = m._malloc(data.length);
	const out = new m.OutputString();
	const dec = new m.VCDiffStreamingDecoder();
	try {
		m.HEAPU8.set(dict, dictPtr);
		m.HEAPU8.set(data, dataPtr);

		dec.StartDecoding(dictPtr, dict.length);
		const ok = dec.DecodeChunkToInterface(dataPtr, data.length, out)
			&& dec.FinishDecoding();

		console.log(ok, out.data(), out.size());
	} finally {
		m.destroy(out);
		m.destroy(dec);
		m._free(dataPtr);
		m._free(dictPtr);
	}
}

Module().then(m => moduleLoaded(m).catch(console.error));
