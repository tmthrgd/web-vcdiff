import Module from './vcddec.js';
import Decoder from './decoder.js';

const asUint8Array = async resp => new Uint8Array(await resp.arrayBuffer());

const m = new Promise(resolve => {
	// See github.com/kripken/emscripten/issues/5820.
	Module({
		locateFile(path, scriptDirectory) {
			// path will be vcddec.wasm and import.meta.url
			// is the url of the current module. By using
			// URL we can convert the relative path into an
			// absolute URL so that the wasm file doesn't
			// need to be stored at the root.
			return new URL(path, import.meta.url).href;
		},
	}).then(Module => resolve({ Module }));
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
	const respBody = asUint8Array(resp);

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

const dictionaryHandlerPath = '/.well-known/web-vcdiff/d/';

const loadDict = async (identifier, base, integrity) => {
	const url = new URL(dictionaryHandlerPath + identifier, base);
	const req = new Request(url, {
		cache: 'force-cache',
		redirect: 'error',
		integrity,
	});

	const resp = await fetch(req);
	if (!resp.ok) {
		throw new Error('failed to load ' + resp.url + ': ' + resp.statusText);
	}

	return await asUint8Array(resp);
};

const decode = async resp => {
	const encHdr = resp.headers.get('Content-Diff-Encoding');
	if (!encHdr || encHdr.toLowerCase() !== 'vcdiff') {
		return resp;
	}

	const identifier = resp.headers.get('Content-Diff-Dictionary');
	if (!identifier || identifier.startsWith(';')) {
		throw new Error('missing Content-Diff-Dictionary header for ' + resp.url);
	}

	const parts = identifier.split(';', 2);
	const dict = loadDict(parts[0], resp.url, parts[1] || '');
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

export { request, decode, vcdiffFetch as fetch };
