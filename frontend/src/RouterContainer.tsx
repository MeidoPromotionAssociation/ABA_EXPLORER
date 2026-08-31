import React from "react";
import {BrowserRouter} from "react-router-dom";
import App from "./App";

// RouterContainer 把路由上下文放在 App 外层，App 内部才能使用 useNavigate
// RouterContainer keeps the router context outside App so App can use useNavigate
const RouterContainer: React.FC = () => (
    <BrowserRouter>
        <App/>
    </BrowserRouter>
);

export default RouterContainer;
