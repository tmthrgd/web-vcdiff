#include <stddef.h>

typedef void *HashedDictionaryPtr;

HashedDictionaryPtr NewHashedDictionary(const char *, size_t);

void DeleteHashedDictionary(HashedDictionaryPtr);

typedef void *VCDiffStreamingEncoderPtr;
typedef int VCDiffFormatExtensionFlags;

VCDiffStreamingEncoderPtr NewVCDiffStreamingEncoder(const HashedDictionaryPtr,
                                                    VCDiffFormatExtensionFlags,
                                                    int, int);

int VCDiffStreamingEncoderEncodeChunk(VCDiffStreamingEncoderPtr, int,
                                      const char *, size_t);

int VCDiffStreamingEncoderFinishEncoding(VCDiffStreamingEncoderPtr, int);