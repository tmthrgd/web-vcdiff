import Module from '/vcddec.js';
import Decoder from '/decoder.js';

const must200 = resp => {
	if (!resp.ok) {
		throw new Error('failed to load ' + resp.url + ': ' + resp.statusText);
	}

	return resp;
};

const asUint8Array = async resp => new Uint8Array(await resp.arrayBuffer());

const m = new Promise(resolve => {
	// See github.com/kripken/emscripten/issues/5820.
	Module().then(Module => resolve({ Module }));
});

const decodeStream = (resp, dict) => new ReadableStream({
	async start(controller) {
		const dec = new Decoder((await m).Module, {
			append(data) {
				controller.enqueue(data.slice());
			},
		});
		try {
			dec.start(await dict);

			const reader = resp.body.getReader();
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
		resp.body && resp.body.cancel(reason);
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

const dictFetch = fetch('/test.dict').then(must200).then(asUint8Array);

const decode = async resp => {
	const encHdr = resp.headers.get('Content-Diff-Encoding');
	if (!encHdr || encHdr.toLowerCase() !== 'vcdiff') {
		return resp;
	}

	const body = resp.body ? decodeStream(resp, dictFetch) : await decodeBuffer(resp, dictFetch);

	resp = new Response(body, resp);
	resp.headers.delete('Content-Diff-Encoding');
	return resp;
};

const request = (input, init) => {
	const req = new Request(input, init);
	req.headers.set('Accept-Diff-Encoding', 'vcdiff');
	return req;
};

const vcdiffFetch = (input, init) => fetch(request(input, init)).then(decode);

export { request, decode, vcdiffFetch as fetch };
