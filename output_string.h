#include <string>

#include <google/output_string.h>

class OutputString : public open_vcdiff::OutputStringInterface {
public:
	virtual OutputString& append(const char* s, size_t n) {
		data_.append(s, n);
		return *this;
	}

	virtual void clear() {
		data_.clear();
	}

	virtual void push_back(char c) {
		data_.push_back(c);
	}

	virtual void ReserveAdditionalBytes(size_t res_arg) {
		data_.reserve(data_.size() + res_arg);
	}

	virtual size_t size() const {
		return data_.size();
	}

	const char *data() const {
		return data_.data();
	}

private:
	std::string data_;
};
