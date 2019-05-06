const debug = true;

const debugAppend = (cb, data) => {
	// The appendCallback passed to constructor cannot retain the memory
	// passed to it as it is only valid for the lifetime of the call. This
	// is an optional debug function that passes a copy of the memory to
	// the callback and zeros it afterwards to ensure the memory is either
	// used or copied by the time the callback returns.

	const copy = data.slice();
	cb(copy);

	for (let i = 0; i < copy.length; i++) {
		copy[i] = 0;
	}
};

export default class {
	constructor(m, cb) {
		if (!m || !m.VCDiffStreamingDecoder) {
			throw new Error('m must be WASM Module');
		}

		this._m = m;

		this._dec = new m.VCDiffStreamingDecoder();
		this._dec.SetMaximumTargetFileSize(0x7fffffff);

		const out = new m.OutputJS();
		out.appendCallback = (s, n) => {
			const data = this._heap.subarray(s, s + n);
			debug ? debugAppend(cb.append, data) : cb.append(data);
		};
		out.reserveCallback = cb.reserve || (n => { /* NOOP */ });
		this._out = out;
	}

	get _heap() { return this._m.HEAPU8; }

	start(dict) {
		if (!this._m) {
			throw new Error('start cannot be called after destroy');
		}
		if (this._dictPtr) {
			throw new Error('start must only be called once');
		}

		const ptr = this._m._malloc(dict.length);
		this._heap.set(dict, ptr);
		this._dictPtr = ptr;

		this._dec.StartDecoding(ptr, dict.length);
	}

	decode(chunk) {
		if (!this._m) {
			throw new Error('decode cannot be called after destroy');
		}
		if (!this._dictPtr) {
			throw new Error('start must called before decode');
		}
		if (!this._dec) {
			throw new Error('decode cannot be called after decode failure');
		}

		let ptr = this._chunkPtr;
		if (!ptr || this._chunkLen < chunk.length) {
			ptr && this._m._free(ptr);
			ptr = this._m._malloc(chunk.length);
			this._chunkPtr = ptr;
			this._chunkLen = chunk.length;
		}

		this._heap.set(chunk, ptr);

		if (!this._dec.DecodeChunkToInterface(ptr, chunk.length, this._out)) {
			this._m.destroy(this._dec);
			this._dec = null;

			throw new Error('DecodeChunkToInterface failed');
		}
	}

	finish() {
		if (!this._m) {
			throw new Error('finish cannot be called after destroy');
		}
		if (!this._dec) {
			throw new Error('finish cannot be called after decode failure');
		}

		if (!this._dec.FinishDecoding()) {
			throw new Error('FinishDecoding failed');
		}
	}

	destroy() {
		this._chunkPtr && this._m._free(this._chunkPtr);
		this._chunkPtr = 0;

		this._dictPtr && this._m._free(this._dictPtr);
		this._dictPtr = 0;

		this._out && this._m.destroy(this._out);
		this._out = null;

		this._dec && this._m.destroy(this._dec);
		this._dec = null;

		this._m = null;
	}
};