// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

import Vue from 'vue';
import App from './App.vue';
import router from './router';
import store from './store';
import { AnalyticsPlugin } from './plugins/analytics';

Vue.config.productionTip = false;
declare module 'vue/types/vue' {
    interface Vue {
        analytics: analytics;
    }
}

Vue.use(AnalyticsPlugin);

new Vue({
    router,
    store,
    render: (h) => h(App),
}).$mount('#app');
