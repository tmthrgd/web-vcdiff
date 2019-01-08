#include <string>

#include <google/output_string.h>

class OutputJSCallbackBase {
public:
	virtual ~OutputJSCallbackBase() {};

	virtual void append(const char* s, unsigned int n) = 0;
};

class OutputJS : public open_vcdiff::OutputStringInterface {
public:
	OutputJS(OutputJSCallbackBase *callbacks) : callbacks_(callbacks) {}

	virtual OutputJS& append(const char* s, size_t n) {
		callbacks_->append(s, n);
		return *this;
	}

	virtual void clear() { abort(); }

	virtual void push_back(char) { abort(); }

	virtual void ReserveAdditionalBytes(size_t) { /* NOOP */ }

	virtual size_t size() const { abort(); }

private:
	OutputJSCallbackBase *callbacks_;
};
