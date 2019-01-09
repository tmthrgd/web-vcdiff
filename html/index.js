import Module from '/vcddec.js';
import Decoder from '/decoder.js';

const textDec = new TextDecoder('utf-8');

const dictFetch = fetch('/test.dict');

async function moduleLoaded(m) {
	const dictResp = await dictFetch;
	if (!dictResp.ok) {
		throw new Error('failed to load /test.dict: ' + dictResp.statusText);
	}

	const dict = new Uint8Array(await dictResp.arrayBuffer());
	const data = new Uint8Array([214, 195, 196, 0, 0, 1, 45, 0, 8, 44, 0, 0, 2, 1, 115, 44, 0]);

	const dec = new Decoder(m, s => {
		console.log(textDec.decode(s), s.length);
	});
	try {
		dec.start(dict);
		dec.decode(data);
		dec.finish();
	} finally {
		dec.destroy();
	}
}

Module().then(m => moduleLoaded(m).catch(console.error));
