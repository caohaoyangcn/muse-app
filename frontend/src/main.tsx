import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import { applyZoom, readZoom } from './utils/zoom'
import { configureClient } from './services/client'
import { wailsClient } from './services/client.wails'
import App from './App'
import ErrorBoundary from './components/ErrorBoundary'

// Bind the Wails-backed transport before anything renders. Views read through
// services/client.ts, so they never touch the generated bindings directly.
configureClient(wailsClient)

// Apply the saved UI scale before the first paint so a remembered zoom does
// not flash at 100%.
applyZoom(readZoom())

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <ErrorBoundary scope="Muse Video" variant="page">
            <App/>
        </ErrorBoundary>
    </React.StrictMode>
)
