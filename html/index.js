import * as vcdiff from '/vcdiff.js';

const must200 = resp => {
	if (!resp.ok) {
		throw new Error('failed to load ' + resp.url + ': ' + resp.statusText);
	}

	return resp;
};

vcdiff.fetch('/test.txt')
	.then(must200)
	.then(resp => resp.text())
	.then(body => {
		document.querySelector('.output').innerText = body;
	})
	.catch(console.error);
