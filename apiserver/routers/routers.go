package routers

import (
	"io"
	"net/http"

	"coriolis-veeam-bridge/apiserver/controllers"

	gorillaHandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// NewAPIRouter returns a new gorilla mux router.
func NewAPIRouter(han *controllers.APIController, logWriter io.Writer) *mux.Router {
	router := mux.NewRouter()
	log := gorillaHandlers.CombinedLoggingHandler

	apiSubRouter := router.PathPrefix("/api/v1").Subrouter()

	// Private API endpoints
	apiRouter := apiSubRouter.PathPrefix("").Subrouter()

	// list disks
	apiRouter.Handle("/disks", log(logWriter, http.HandlerFunc(han.ListDisksHandler))).Methods("GET")
	apiRouter.Handle("/disks/", log(logWriter, http.HandlerFunc(han.ListDisksHandler))).Methods("GET")

	// View one disk. Only disks added to tracking can be viewed here.
	apiRouter.Handle("/disks/{diskTrackingID}", log(logWriter, http.HandlerFunc(han.GetDiskHandler))).Methods("GET")
	apiRouter.Handle("/disks/{diskTrackingID}/", log(logWriter, http.HandlerFunc(han.GetDiskHandler))).Methods("GET")

	// Create/view snap stores for a disk
	apiRouter.Handle("/disks/{diskTrackingID}/snapstore", log(logWriter, http.HandlerFunc(han.ListDisksHandler))).Methods("GET")
	apiRouter.Handle("/disks/{diskTrackingID}/snapstore/", log(logWriter, http.HandlerFunc(han.ListDisksHandler))).Methods("GET")

	// View single disk snapshots. This is read only. Any create/delete operations needs to be done
	// using the /snapshots endpoint. A snapshot can encompass multiple disks.
	apiRouter.Handle("/disks/{diskTrackingID}/snapshots", log(logWriter, http.HandlerFunc(han.ListDisksHandler))).Methods("GET")
	apiRouter.Handle("/disks/{diskTrackingID}/snapshots/", log(logWriter, http.HandlerFunc(han.ListDisksHandler))).Methods("GET")

	///////////////
	// Snapshots //
	///////////////
	// view or delete a single snapshot.
	apiRouter.Handle("/snapshots/{snapshotID}", log(logWriter, http.HandlerFunc(han.GetSnapshotHandler))).Methods("GET")
	apiRouter.Handle("/snapshots/{snapshotID}/", log(logWriter, http.HandlerFunc(han.GetSnapshotHandler))).Methods("GET")

	apiRouter.Handle("/snapshots/{snapshotID}", log(logWriter, http.HandlerFunc(han.DeleteSnapshotHandler))).Methods("DELETE")
	apiRouter.Handle("/snapshots/{snapshotID}/", log(logWriter, http.HandlerFunc(han.DeleteSnapshotHandler))).Methods("DELETE")

	// Create and view snapshots endpoint.
	apiRouter.Handle("/snapshots", log(logWriter, http.HandlerFunc(han.ListSnapshotsHandler))).Methods("GET")
	apiRouter.Handle("/snapshots/", log(logWriter, http.HandlerFunc(han.ListSnapshotsHandler))).Methods("GET")

	apiRouter.Handle("/snapshots", log(logWriter, http.HandlerFunc(han.CreateSnapshotHandler))).Methods("POST")
	apiRouter.Handle("/snapshots/", log(logWriter, http.HandlerFunc(han.CreateSnapshotHandler))).Methods("POST")

	apiRouter.Handle("/snapshots/{snapshotID}", log(logWriter, http.HandlerFunc(han.DeleteSnapshotHandler))).Methods("DELETE")
	apiRouter.Handle("/snapshots/{snapshotID}/", log(logWriter, http.HandlerFunc(han.DeleteSnapshotHandler))).Methods("DELETE")

	apiRouter.Handle("/snapshots/{snapshotID}/changes/{trackedDiskID}", log(logWriter, http.HandlerFunc(han.GetChangedSectorsHandler))).Methods("GET")
	apiRouter.Handle("/snapshots/{snapshotID}/changes/{trackedDiskID}/", log(logWriter, http.HandlerFunc(han.GetChangedSectorsHandler))).Methods("GET")

	apiRouter.Handle("/snapshots/{snapshotID}/consume/{trackedDiskID}", log(logWriter, http.HandlerFunc(han.ConsumeSnapshotHandler))).Methods("GET", "HEAD")
	apiRouter.Handle("/snapshots/{snapshotID}/consume/{trackedDiskID}/", log(logWriter, http.HandlerFunc(han.ConsumeSnapshotHandler))).Methods("GET", "HEAD")

	// snap store management.
	// Read snap stores
	apiRouter.Handle("/snapstores", log(logWriter, http.HandlerFunc(han.ListSnapStoreHandler))).Methods("GET")
	apiRouter.Handle("/snapstores/", log(logWriter, http.HandlerFunc(han.ListSnapStoreHandler))).Methods("GET")
	// Create snap store
	// apiRouter.Handle("/snapstores", log(logWriter, http.HandlerFunc(han.CreateSnapStoreHandler))).Methods("POST")
	// apiRouter.Handle("/snapstores/", log(logWriter, http.HandlerFunc(han.CreateSnapStoreHandler))).Methods("POST")

	apiRouter.Handle("/snapstorelocations", log(logWriter, http.HandlerFunc(han.ListSnapStoreLocations))).Methods("GET")
	apiRouter.Handle("/snapstorelocations/", log(logWriter, http.HandlerFunc(han.ListSnapStoreLocations))).Methods("GET")

	// snap store mappings
	apiRouter.Handle("/snapstoremappings", log(logWriter, http.HandlerFunc(han.ListSnapStoreMappingsHandler))).Methods("GET")
	apiRouter.Handle("/snapstoremappings/", log(logWriter, http.HandlerFunc(han.ListSnapStoreMappingsHandler))).Methods("GET")

	apiRouter.Handle("/snapstoremappings", log(logWriter, http.HandlerFunc(han.CreateSnapStoreMappingHandler))).Methods("POST")
	apiRouter.Handle("/snapstoremappings/", log(logWriter, http.HandlerFunc(han.CreateSnapStoreMappingHandler))).Methods("POST")

	apiRouter.Handle("/snapstoremappings/{mappingID}", log(logWriter, http.HandlerFunc(han.ListSnapStoreHandler))).Methods("DELETE")
	apiRouter.Handle("/snapstoremappings/{mappingID}/", log(logWriter, http.HandlerFunc(han.ListSnapStoreHandler))).Methods("DELETE")

	// Not found handler
	apiRouter.PathPrefix("/").Handler(log(logWriter, http.HandlerFunc(han.NotFoundHandler)))

	return router
}
