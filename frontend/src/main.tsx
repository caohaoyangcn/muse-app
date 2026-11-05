import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import ErrorBoundary from './components/ErrorBoundary'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <ErrorBoundary scope="Muse Video" variant="page">
            <App/>
        </ErrorBoundary>
    </React.StrictMode>
)
