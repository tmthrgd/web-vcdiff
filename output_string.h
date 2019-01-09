#include <google/output_string.h>

class OutputJSBase : public open_vcdiff::OutputStringInterface {
public:
	virtual OutputJSBase& append(const char* s, size_t n) {
		appendCallback(s, n);
		return *this;
	}

	virtual void clear() { abort(); }

	virtual void push_back(char) { abort(); }

	virtual void ReserveAdditionalBytes(size_t) { /* NOOP */ }

	virtual size_t size() const { abort(); }

protected:
	virtual void appendCallback(const char* s, unsigned int n) = 0;
};
