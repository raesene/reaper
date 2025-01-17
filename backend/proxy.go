package backend

import (
	"context"
	"net/http"

	"github.com/ghostsecurity/reaper/backend/packaging"
	"github.com/ghostsecurity/reaper/backend/proxy"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) StopProxy() {
	if err := a.stopProxy(); err != nil {
		a.logger.Errorf("Failed to stop proxy: %s", err)
	}
}

func (a *App) stopProxy() error {
	if a.proxy != nil {
		a.logger.Infof("Stopping proxy...")
		if err := a.proxy.Close(); err != nil {
			return err
		}
		a.logger.Infof("Proxy stopped.")
	} else {
		a.logger.Infof("Proxy is already stopped.")
	}
	return nil
}

func (a *App) StartProxy() {
	go a.startProxy()
}

func (a *App) startProxy() {

	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()

	// create a new proxy with current user settings, and start it
	// errors should be emitted using proxy status events

	if a.proxy != nil {
		if err := a.stopProxy(); err != nil {
			a.logger.Errorf("Failed to stop proxy: %s", err)
			a.notifyUser("Failed to stop proxy: "+err.Error(), runtime.ErrorDialog)
			a.Shutdown(context.Background())
			return
		}
	}

	provider := a.userSettings

	a.logger.Infof("Creating proxy...")
	var err error
	a.proxy, err = proxy.New(provider, a.logger.WithPrefix("proxy"))
	if err != nil {
		a.logger.Errorf("Failed to create proxy: %s", err)
		a.emitProxyStatus(false, "", "Proxy creation failed: "+err.Error())
		return
	}

	a.logger.Infof("Setting up proxy handlers...")
	a.proxy.OnRequest(func(request *http.Request, _ int64) (*http.Request, *http.Response) {
		if request.Host == provider.Get().ProxyHost {
			return request, a.handleLocalRequest(request)
		}
		return request, nil
	})
	a.proxy.OnRequest(func(request *http.Request, id int64) (*http.Request, *http.Response) {
		a.workspaceMu.RLock()
		defer a.workspaceMu.RUnlock()
		if !a.workspace.Scope.Includes(request) {
			return request, nil
		}
		if packaged, err := packaging.PackageHttpRequest(request, a.proxy.ID(), id); err != nil {
			a.logger.Errorf("Error packaging request: %s", err)
		} else {
			runtime.EventsEmit(a.ctx, EventHttpRequest, packaged)
		}
		// update workspace tree
		tree, changed := a.workspace.UpdateTree(request)

		if changed { // TODO: do we really want to save changes after every tree change?
			runtime.EventsEmit(a.ctx, EventTreeUpdate, tree.Structure())
			if err := a.workspace.Save(); err != nil {
				a.logger.Errorf("Failed to save workspace after tree change: %s", err)
			}
		}

		a.logger.Debugf("Request %d in scope: %s %s", id, request.Method, request.URL)
		return a.interceptor.Intercept(request, id)
	})
	a.proxy.OnResponse(func(response *http.Response, id int64) *http.Response {
		if response == nil {
			return nil
		}
		a.workspaceMu.RLock()
		defer a.workspaceMu.RUnlock()
		if !a.workspace.Scope.Includes(response.Request) {
			a.logger.Debugf("Response %d NOT in scope: %s %s %d", id, response.Request.Method, response.Request.URL, response.StatusCode)
			return response
		}
		a.logger.Debugf("Response %d in scope: %s %s %d", id, response.Request.Method, response.Request.URL, response.StatusCode)
		if packaged, err := packaging.PackageHttpResponse(response, a.proxy.ID(), id); err != nil {
			a.logger.Errorf("Error packaging response: %s", err)
		} else {
			runtime.EventsEmit(a.ctx, EventHttpResponse, packaged)
		}
		return response
	})

	a.emitProxyStatus(true, a.proxy.Addr(), "")

	a.logger.Infof("Starting proxy...")
	if err := a.proxy.Run(); err != nil {
		a.logger.Errorf("Failed to start proxy: %s", err)
		a.emitProxyStatus(false, a.proxy.Addr(), "Proxy start failed: "+err.Error())
	}

	a.emitProxyStatus(false, a.proxy.Addr(), "Proxy is not running")
	a.logger.Infof("Proxy shut down cleanly.")
}

func (a *App) restartProxy() error {
	if err := a.stopProxy(); err != nil {
		return err
	}
	go a.startProxy()
	return nil
}
