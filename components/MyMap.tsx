"use client";

import { Map, MapControls } from "@/components/ui/map"; 
import { Card } from "@/components/ui/card"; 

export function MyMap() { 
  return ( 
    <Card className="h-full w-full p-0 overflow-hidden relative border-0 shadow-sm rounded-xl"> 
      <Map center={[-74.006, 40.7128]} zoom={11}> 
        <MapControls /> 
      </Map> 
    </Card> 
  ); 
}
