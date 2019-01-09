import Module from '/vcddec.js';
import Decoder from '/decoder.js';

const textDec = new TextDecoder('utf-8');

const must200 = resp => {
	if (!resp.ok) {
		throw new Error('failed to load ' + resp.url + ': ' + resp.statusText);
	}

	return resp;
};
const asUint8Array = async resp => new Uint8Array(await resp.arrayBuffer());

const dictFetch = fetch('/test.dict').then(must200).then(asUint8Array);
const dataFetch = fetch('/test.diff').then(must200).then(asUint8Array);

async function moduleLoaded(m) {
	const dec = new Decoder(m, s => {
		console.log(textDec.decode(s), s.length);
	});
	try {
		dec.start(await dictFetch);
		dec.decode(await dataFetch);
		dec.finish();
	} finally {
		dec.destroy();
	}
}

Module().then(m => moduleLoaded(m).catch(console.error));
