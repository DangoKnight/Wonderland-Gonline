import { createApp } from "vue";
import App from "./App.vue";
import Viewer from "./Viewer.vue";
import "./style.css";

createApp(location.pathname.startsWith("/viewer/") ? Viewer : App).mount("#app");
