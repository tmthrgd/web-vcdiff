#include <cstdlib>
#include <google/vcdecoder.h>

class OutputJSBase : public open_vcdiff::OutputStringInterface {
      public:
	virtual OutputJSBase &append(const char *s, size_t n) {
		appendCallback(s, n);
		return *this;
	}

	virtual void clear() { std::abort(); }

	virtual void push_back(char) { std::abort(); }

	virtual void ReserveAdditionalBytes(size_t res_arg) {
		reserveCallback(res_arg);
	}

	virtual size_t size() const { std::abort(); }

      protected:
	virtual void appendCallback(const char *s, unsigned int n) = 0;
	virtual void reserveCallback(unsigned int n) = 0;
};

#include <vcddec_glue.cpp>
