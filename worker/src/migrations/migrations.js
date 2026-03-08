import journal from './meta/_journal.json';
import m0000 from './0000_simple_stingray.sql' with { type: 'text' };

export default {
	journal,
	migrations: {
		m0000,
	},
};
