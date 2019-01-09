export default class {
	constructor(m, cb) {
		if (!m || !m.VCDiffStreamingDecoder) {
			throw new Error('m must be WASM Module');
		}

		this._m = m;
		this._dec = new m.VCDiffStreamingDecoder();

		const out = new m.OutputJS();
		out.appendCallback = (s, n) => cb(this._heap.subarray(s, s + n));
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

		let ptr = this._chunkPtr;
		if (!ptr || this._chunkLen < chunk.length) {
			ptr && this._m._free(ptr);
			ptr = this._m._malloc(chunk.length);
			this._chunkPtr = ptr;
			this._chunkLen = chunk.length;
		}

		this._heap.set(chunk, ptr);

		if (!this._dec.DecodeChunkToInterface(ptr, chunk.length, this._out)) {
			throw new Error('DecodeChunkToInterface failed');
		}
	}

	finish() {
		if (!this._m) {
			throw new Error('finish cannot be called after destroy');
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