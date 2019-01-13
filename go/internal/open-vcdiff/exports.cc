#include <cstdlib>
#include <google/vcencoder.h>

extern "C" void goOpenVCDIFFWriterWrite(int, const char *, size_t);

class OutputGo : public open_vcdiff::OutputStringInterface {
      public:
	OutputGo(int writer_idx) : writer_idx_(writer_idx) {}

	virtual OutputGo &append(const char *s, size_t n) {
		goOpenVCDIFFWriterWrite(writer_idx_, s, n);
		size_ += n;
		return *this;
	}

	virtual void clear() {} /* NOOP */

	virtual void push_back(char c) { append(&c, 1); }

	virtual void ReserveAdditionalBytes(size_t) {} /* NOOP */

	virtual size_t size() const { return size_; }

      private:
	int writer_idx_;
	size_t size_ = 0;
};

extern "C" {
#include "exports.h"

HashedDictionaryPtr NewHashedDictionary(const char *dictionary_contents,
                                        size_t dictionary_size) {
	auto obj = new open_vcdiff::HashedDictionary(dictionary_contents,
	                                             dictionary_size);
	if (!obj->Init()) {
		delete obj;
		return nullptr;
	}

	return static_cast<HashedDictionaryPtr>(obj);
}

void DeleteHashedDictionary(HashedDictionaryPtr ptr) {
	delete static_cast<open_vcdiff::HashedDictionary *>(ptr);
}

VCDiffStreamingEncoderPtr
NewVCDiffStreamingEncoder(const HashedDictionaryPtr dictionary_ptr,
                          VCDiffFormatExtensionFlags format_extensions,
                          int look_for_target_matches, int writer_idx) {
	auto dictionary =
	    static_cast<const open_vcdiff::HashedDictionary *>(dictionary_ptr);
	auto obj = new open_vcdiff::VCDiffStreamingEncoder(
	    dictionary, format_extensions, look_for_target_matches != 0);

	OutputGo out(writer_idx);
	if (!obj->StartEncodingToInterface(&out)) {
		delete obj;
		return nullptr;
	}

	return static_cast<VCDiffStreamingEncoderPtr>(obj);
}

int VCDiffStreamingEncoderEncodeChunk(VCDiffStreamingEncoderPtr ptr,
                                      int writer_idx, const char *data,
                                      size_t n) {
	auto obj = static_cast<open_vcdiff::VCDiffStreamingEncoder *>(ptr);

	OutputGo out(writer_idx);
	return obj->EncodeChunkToInterface(data, n, &out);
}

int VCDiffStreamingEncoderFinishEncoding(VCDiffStreamingEncoderPtr ptr,
                                         int writer_idx) {
	auto obj = static_cast<open_vcdiff::VCDiffStreamingEncoder *>(ptr);

	OutputGo out(writer_idx);
	auto ok = obj->FinishEncodingToInterface(&out);

	delete obj;
	return ok;
}
}