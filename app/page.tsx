import { MyMap } from "@/components/MyMap";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { MapPin, Navigation, Activity } from "lucide-react";

export default function Home() {
  return (
    <div className="min-h-screen bg-neutral-950 text-neutral-50 font-sans selection:bg-neutral-800">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-10">
        
        {/* Header */}
        <header className="mb-10">
          <div className="flex items-center gap-3">
            <div className="bg-emerald-500/10 p-3 rounded-2xl border border-emerald-500/20">
              <Navigation className="w-6 h-6 text-emerald-400" />
            </div>
            <div>
              <h1 className="text-3xl font-bold tracking-tight text-white">Location Tracker</h1>
              <p className="text-neutral-400 mt-1">Real-time asset positioning and analytics</p>
            </div>
          </div>
        </header>

        {/* Main Grid */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          
          {/* Left Column - Stats & Info */}
          <div className="space-y-8 flex flex-col">
            <div className="grid grid-cols-2 gap-4">
              <Card className="bg-neutral-900 border-neutral-800 text-white shadow-xl shadow-black/50">
                <CardHeader className="pb-2">
                  <CardDescription className="text-neutral-400 font-medium">Active Assets</CardDescription>
                  <CardTitle className="text-4xl font-light">124</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-sm text-emerald-400 flex items-center gap-1">
                    <Activity className="w-4 h-4" /> +12% this week
                  </div>
                </CardContent>
              </Card>
              <Card className="bg-neutral-900 border-neutral-800 text-white shadow-xl shadow-black/50">
                <CardHeader className="pb-2">
                  <CardDescription className="text-neutral-400 font-medium">Alerts</CardDescription>
                  <CardTitle className="text-4xl font-light">3</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-sm text-red-400 flex items-center gap-1">
                    Needs attention
                  </div>
                </CardContent>
              </Card>
            </div>

            <Card className="flex-1 bg-neutral-900 border-neutral-800 text-white shadow-xl shadow-black/50">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg">
                  <MapPin className="w-5 h-5 text-emerald-400" />
                  Recent Activity
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                {[
                  { name: "Vehicle 042", loc: "Brooklyn, NY", time: "2 min ago" },
                  { name: "Drone 17", loc: "Manhattan, NY", time: "5 min ago" },
                  { name: "Asset X", loc: "Queens, NY", time: "12 min ago" },
                ].map((item, i) => (
                  <div key={i} className="flex items-center justify-between p-3 rounded-lg bg-neutral-950/50 border border-neutral-800/50">
                    <div>
                      <div className="font-medium">{item.name}</div>
                      <div className="text-sm text-neutral-500">{item.loc}</div>
                    </div>
                    <div className="text-xs text-neutral-400">{item.time}</div>
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>

          {/* Right Column - Map */}
          <div className="lg:col-span-2 flex flex-col">
            <Card className="flex-1 bg-neutral-900 border-neutral-800 text-white shadow-xl shadow-black/50 overflow-hidden flex flex-col min-h-[500px]">
              <CardHeader className="bg-neutral-900 border-b border-neutral-800 shrink-0">
                <CardTitle className="text-lg">Live Map View</CardTitle>
                <CardDescription className="text-neutral-400">Tracking assets in real-time across the region</CardDescription>
              </CardHeader>
              <CardContent className="p-0 flex-1 relative">
                <div className="absolute inset-0">
                  <MyMap />
                </div>
              </CardContent>
            </Card>
          </div>

        </div>
      </div>
    </div>
  );
}
