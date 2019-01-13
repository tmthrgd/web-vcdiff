import Module from '/vcddec.js';
import Decoder from '/decoder.js';

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

const decodeBuffer = async (resp, dict) => {
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
		dec.decode(await asUint8Array(resp));
		dec.finish();
	} finally {
		dec.destroy();
	}

	return body.subarray(0, pos);
};

const dictCache = caches.open('vcdiff-dict');

const loadDict = async header => {
	const cache = await dictCache;

	const url = new URL(header, location);
	const hash = url.hash && url.hash.slice(1);
	url.hash = '';
	const req = new Request(url, {
		cache: 'no-store',
		headers: hash ? { 'Expect-Diff-Hash': hash } : {},
	});

	const entry = await cache.match(req);
	if (entry) {
		return entry;
	}

	const resp = await fetch(req);
	if (!resp.ok) {
		throw new Error('failed to load ' + resp.url + ': ' + resp.statusText);
	}

	cache.put(req, resp.clone()).catch(err => {
		console.error('failed to store dictionary in cache:', err);
	});
	return resp;
};

const flushCache = async () => {
	const cache = await dictCache;
	const keys = await cache.keys();
	await Promise.all(keys.map(req => cache.delete(req)));
};

const decode = async resp => {
	const encHdr = resp.headers.get('Content-Diff-Encoding');
	if (!encHdr || encHdr.toLowerCase() !== 'vcdiff') {
		return resp;
	}

	const dictHdr = resp.headers.get('Content-Diff-Dictionary');
	if (!dictHdr) {
		throw new Error('missing Content-Diff-Dictionary header for ' + resp.url);
	}

	const dict = loadDict(dictHdr).then(asUint8Array);
	const body = resp.body ? decodeStream(resp.body, dict) : await decodeBuffer(resp, dict);

	resp = new Response(body, resp);
	resp.headers.delete('Content-Diff-Encoding');
	resp.headers.delete('Content-Diff-Dictionary');
	return resp;
};

const request = (input, init) => {
	const req = new Request(input, init);
	req.headers.set('Accept-Diff-Encoding', 'vcdiff');
	return req;
};

const vcdiffFetch = (input, init) => fetch(request(input, init)).then(decode);

export { request, decode, vcdiffFetch as fetch, flushCache };
