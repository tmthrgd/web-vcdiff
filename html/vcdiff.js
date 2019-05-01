import Module from '/vcddec.js';
import Decoder from '/decoder.js';
import * as idb from '/idb-keyval.js';

const asUint8Array = async resp => new Uint8Array(await resp.arrayBuffer());

const m = new Promise(resolve => {
	// See github.com/kripken/emscripten/issues/5820.
	Module().then(Module => resolve({ Module }));
});

const decodeStream = (body, dict) => new ReadableStream({
	async start(controller) {
		const dec = new Decoder((await m).Module, {
			append(data) {
				controller.enqueue(data.slice());
			},
		});
		try {
			dec.start(await dict);

			const reader = body.getReader();
			for (; ;) {
				const { done, value } = await reader.read();
				if (done) {
					break;
				}

				dec.decode(value);
			}

			dec.finish();
		} finally {
			dec.destroy();
		}

		controller.close();
	},

	cancel(reason) {
		body.cancel(reason);
	},
});

const decodeBuffer = async (respBody, dict) => {
	let body = new Uint8Array(0);
	let pos = 0;
	const reserve = n => {
		if (body.length - pos >= n) {
			return;
		}

		const tmp = new Uint8Array(body.length + n);
		tmp.set(body);
		body = tmp;
	};

	const dec = new Decoder((await m).Module, {
		append(data) {
			reserve(data.length);
			body.set(data, pos);
			pos += data.length;
		},

		reserve,
	});
	try {
		dec.start(await dict);
		dec.decode(await respBody);
		dec.finish();
	} finally {
		dec.destroy();
	}

	return body.subarray(0, pos);
};

const store = new idb.Store('vcdiff', 'dict-cache');

const flushCache = () => idb.clear(store);

const readFile = file => new Promise((resolve, reject) => {
	const fr = new FileReader();
	fr.onload = () => resolve(new Uint8Array(fr.result));
	fr.onerror = () => reject(fr.error);
	fr.readAsArrayBuffer(file);
});

const decode = async resp => {
	const encHdr = resp.headers.get('Content-Diff-Encoding');
	if (!encHdr || !encHdr.toLowerCase().startsWith('vcdiff:')) {
		return resp;
	}

	const dictHash = encHdr.slice('vcdiff:'.length);
	if (!dictHash || dictHash === '*') {
		throw new Error('missing dictionary hash from Content-Diff-Encoding header: ' + encHdr);
	}

	let body, newCT;
	if (dictHash.startsWith('*')) {
		const fd = await resp.formData();

		const dict = readFile(fd.get('dict'));
		dict.then(dict => idb.set(dictHash.slice(1), dict, store))
			.catch(err => console.error('failed to cache dictionary ' + encHdr + ':', err));

		const bodyFile = fd.get('body');
		newCT = bodyFile.type;

		body = await decodeBuffer(readFile(bodyFile), dict);
	} else {
		const dict = idb.get(dictHash, store);

		body = resp.body
			? decodeStream(resp.body, dict)
			: await decodeBuffer(asUint8Array(resp), dict);
	}

	resp = new Response(body, resp);
	resp.headers.delete('Content-Diff-Encoding');

	if (newCT) {
		resp.headers.set('Content-Type', newCT);
	}

	return resp;
};

const request = async (input, init) => {
	const req = new Request(input, init);
	req.headers.set('Accept-Diff-Encoding', 'vcdiff');

	const dicts = await idb.keys(store);
	req.headers.set('Accept-Diff-Dictionaries', dicts.join(','));

	return req;
};

const vcdiffFetch = async (input, init) => {
	const req = await request(input, init);
	const resp = await fetch(req);
	return await decode(resp);
};

export { request, decode, vcdiffFetch as fetch, flushCache };
