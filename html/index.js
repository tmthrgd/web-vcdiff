import Module from '/vcddec.js';

const textDec = new TextDecoder("utf-8");

const dictFetch = fetch('/test.dict');

async function moduleLoaded(m) {
	const dictResp = await dictFetch;
	if (!dictResp.ok) {
		throw new Error('failed to load /test.dict: ' + dictResp.statusText);
	}

	const dict = new Uint8Array(await dictResp.arrayBuffer());
	const data = new Uint8Array([214, 195, 196, 0, 0, 1, 45, 0, 8, 44, 0, 0, 2, 1, 115, 44, 0]);

	const outCb = new m.OutputJSCallback();
	outCb.append = (s, n) => {
		const data = m.HEAPU8.subarray(s, s + n);
		const str = textDec.decode(data);
		console.log(str, n);
	};

	const dictPtr = m._malloc(dict.length);
	const dataPtr = m._malloc(data.length);
	const out = new m.OutputJS(outCb);
	const dec = new m.VCDiffStreamingDecoder();
	try {
		m.HEAPU8.set(dict, dictPtr);
		m.HEAPU8.set(data, dataPtr);

		dec.StartDecoding(dictPtr, dict.length);

		if (!dec.DecodeChunkToInterface(dataPtr, data.length, out)) {
			throw new Error('DecodeChunkToInterface failed');
		}

		if (!dec.FinishDecoding()) {
			throw new Error('FinishDecoding failed');
		}
	} finally {
		m.destroy(dec);
		m.destroy(out);
		m.destroy(outCb);
		m._free(dataPtr);
		m._free(dictPtr);
	}
}

Module().then(m => moduleLoaded(m).catch(console.error));
