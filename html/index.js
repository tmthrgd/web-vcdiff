import Module from '/vcddec.js';
import Decoder from '/decoder.js';

const must200 = resp => {
	if (!resp.ok) {
		throw new Error('failed to load ' + resp.url + ': ' + resp.statusText);
	}

	return resp;
};

const asUint8Array = async resp => new Uint8Array(await resp.arrayBuffer());

const decodeResp = async (dec, resp) => {
	if (!resp.body) {
		return dec.decode(await asUint8Array(resp));
	}

	const reader = resp.body.getReader();
	for (; ;) {
		const { done, value } = await reader.read();
		if (done) {
			return;
		}

		dec.decode(value);
	}
};

const dictFetch = fetch('/test.dict').then(must200).then(asUint8Array);
const dataFetch = fetch('/test.diff').then(must200);

async function moduleLoaded(m) {
	const textDec = new TextDecoder('utf-8');

	const dec = new Decoder(m, {
		append(data) {
			const text = textDec.decode(data, { stream: true });
			console.log(text, data.length);
		},
	});
	try {
		dec.start(await dictFetch);
		await decodeResp(dec, await dataFetch);
		dec.finish();
	} finally {
		dec.destroy();
	}

	const last = textDec.decode();
	last && console.log(last);
}

Module().then(m => moduleLoaded(m).catch(console.error));
